package kaggle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/himanshusardana/jupy/internal/notebook"
)

// ErrSessionRunning means Kaggle already has a session for this notebook, so
// the caller has to attach to the existing one instead of starting a new one.
var ErrSessionRunning = errors.New("a session is already running for this notebook")

// Accelerators are the machine shapes Kaggle accepts for a session.
type Accelerator struct {
	Label string
	Shape string
}

var Accelerators = []Accelerator{
	{Label: "cpu", Shape: ""},
	{Label: "T4 x2", Shape: "NvidiaTeslaT4"},
	{Label: "P100", Shape: "NvidiaTeslaP100"},
	{Label: "L4", Shape: "NvidiaL4"},
	{Label: "TPU 1VM v3-8", Shape: "Tpu1VmV38"},
}

type SessionRequest struct {
	Slug         string
	Language     string
	KernelType   string
	MachineShape string
	DockerImage  string
	Internet     *bool

	// Timeout bounds the wait for the operation to finish; Poll is the first
	// poll interval (it backs off up to 10s).
	Timeout time.Duration
	Poll    time.Duration
}

// StartedSession is a session Kaggle created for us, with the Jupyter URLs its
// operation reported.
type StartedSession struct {
	URLs      []string
	SessionID int
}

type operation struct {
	Name     string          `json:"name"`
	Done     bool            `json:"done"`
	Metadata json.RawMessage `json:"metadata"`
	Response json.RawMessage `json:"response"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// StartSession asks Kaggle to start an interactive session for a notebook and
// waits for the long-running operation that reports its Jupyter URL.
//
// Kaggle answers with jupyterUrl and tokenizedJupyterUrl (undocumented shape;
// observed 2026). The tokenized form can be a bare proxy host plus ?token=, so
// the plain URL carrying that token is offered first and the tokenized URL is
// kept as a fallback.
func (c *Client) StartSession(ctx context.Context, req SessionRequest) (StartedSession, error) {
	if req.Slug == "" {
		return StartedSession{}, fmt.Errorf("start session needs a kaggle notebook")
	}
	if req.Timeout <= 0 {
		req.Timeout = 3 * time.Minute
	}
	if req.Poll <= 0 {
		req.Poll = 2 * time.Second
	}

	body := map[string]any{"slug": req.Slug}
	if req.Language != "" {
		body["language"] = req.Language
	}
	if req.KernelType != "" {
		body["kernelType"] = req.KernelType
	}
	if req.MachineShape != "" {
		body["machineShape"] = req.MachineShape
	}
	if req.DockerImage != "" {
		body["dockerImage"] = req.DockerImage
	}
	if req.Internet != nil {
		body["enableInternet"] = *req.Internet
	}

	var op operation
	if err := c.callCtx(ctx, "kernels.KernelsApiService", "CreateKernelSession", body, &op); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Code == 409 {
			return StartedSession{}, ErrSessionRunning
		}
		return StartedSession{}, err
	}

	deadline := time.Now().Add(req.Timeout)
	delay := req.Poll
	for !op.Done && op.Name != "" && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return StartedSession{}, ctx.Err()
		case <-time.After(delay):
		}
		if delay = delay * 3 / 2; delay > 10*time.Second {
			delay = 10 * time.Second
		}

		var next operation
		if err := c.callCtx(ctx, "common.OperationsService", "GetOperation", map[string]any{"name": op.Name}, &next); err != nil {
			return StartedSession{}, err
		}
		next.Name = firstNonEmpty(next.Name, op.Name)
		op = next
	}

	if op.Error != nil && op.Error.Message != "" {
		return StartedSession{}, fmt.Errorf("kaggle could not start a session: %s", op.Error.Message)
	}
	if !op.Done {
		return StartedSession{}, fmt.Errorf("timed out waiting for the session operation %q", op.Name)
	}

	started := StartedSession{
		URLs:      sessionURLs(op.Response),
		SessionID: sessionID(op),
	}
	if len(started.URLs) == 0 {
		// Do not leave a session burning quota that we cannot attach to.
		if started.SessionID != 0 {
			c.CancelSession(started.SessionID)
		}
		return StartedSession{}, fmt.Errorf("kaggle started a session but returned no jupyter url; press C to paste the url from Run > Kaggle Jupyter Server")
	}
	return started, nil
}

// CancelSession stops a session started through the API.
func (c *Client) CancelSession(id int) error {
	if id == 0 {
		return nil
	}
	return c.call("CancelKernelSession", map[string]any{"kernelSessionId": id}, nil)
}

// sessionURLs pulls the Jupyter URLs out of an operation response, preferring a
// plain URL that carries the token over the tokenized bare-host form.
func sessionURLs(raw json.RawMessage) []string {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return nil
	}

	plain := firstString(payload, "jupyterUrl", "jupyter_url")
	tokenized := firstString(payload, "tokenizedJupyterUrl", "tokenized_jupyter_url")

	var out []string
	var token string
	if u, err := url.Parse(tokenized); err == nil {
		token = u.Query().Get("token")
	}
	if plain != "" {
		if token != "" && !strings.Contains(plain, "token=") {
			sep := "?"
			if strings.Contains(plain, "?") {
				sep = "&"
			}
			out = append(out, plain+sep+"token="+url.QueryEscape(token))
		}
		out = append(out, plain)
	}
	if tokenized != "" {
		out = append(out, tokenized)
	}
	if len(out) == 0 {
		if fallback := findProxyURL(payload); fallback != "" {
			out = append(out, fallback)
		}
	}

	seen := map[string]bool{}
	var unique []string
	for _, u := range out {
		if u != "" && !seen[u] {
			seen[u] = true
			unique = append(unique, u)
		}
	}
	return unique
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// findProxyURL walks the payload for anything that looks like a Jupyter proxy,
// in case Kaggle renames the documented-by-observation fields.
func findProxyURL(payload map[string]any) string {
	best, bestScore := "", 0
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch typed := v.(type) {
		case map[string]any:
			for k, child := range typed {
				walk(child, strings.ToLower(k))
			}
		case []any:
			for _, child := range typed {
				walk(child, key)
			}
		case string:
			if !strings.HasPrefix(typed, "http") {
				return
			}
			lower := strings.ToLower(typed)
			score := 0
			for _, hint := range []string{"jupyter", "proxy", "kkb", "token="} {
				if strings.Contains(lower, hint) {
					score++
				}
			}
			if strings.Contains(key, "jupyter") || strings.Contains(key, "proxy") {
				score += 2
			}
			if strings.HasPrefix(lower, "https://www.kaggle.com/code") {
				score -= 3
			}
			if score > bestScore {
				best, bestScore = typed, score
			}
		}
	}
	walk(map[string]any(payload), "")
	if bestScore <= 0 {
		return ""
	}
	return best
}

// sessionID finds the kernel session id: the CreateKernelSession operation puts
// it in metadata ({"kernelSessionId": N}) and in its name
// ("operations/create-kernel-session/N").
func sessionID(op operation) int {
	for _, raw := range []json.RawMessage{op.Metadata, op.Response} {
		if id := findSessionID(raw); id != 0 {
			return id
		}
	}
	const prefix = "operations/create-kernel-session/"
	if strings.HasPrefix(op.Name, prefix) {
		var id int
		if _, err := fmt.Sscanf(strings.TrimPrefix(op.Name, prefix), "%d", &id); err == nil {
			return id
		}
	}
	return 0
}

func findSessionID(raw json.RawMessage) int {
	var payload any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return 0
	}
	var walk func(v any) int
	walk = func(v any) int {
		switch typed := v.(type) {
		case map[string]any:
			for k, child := range typed {
				key := strings.ToLower(strings.ReplaceAll(k, "_", ""))
				if key == "kernelsessionid" || key == "sessionid" {
					switch n := child.(type) {
					case float64:
						return int(n)
					case string:
						var id int
						if _, err := fmt.Sscanf(n, "%d", &id); err == nil {
							return id
						}
					}
				}
				if hit := walk(child); hit != 0 {
					return hit
				}
			}
		case []any:
			for _, child := range typed {
				if hit := walk(child); hit != 0 {
					return hit
				}
			}
		}
		return 0
	}
	return walk(payload)
}

// Slugify turns a notebook title into a Kaggle slug: lowercase, dashes for
// anything that is not a letter or digit.
func Slugify(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// normalizeRef turns whatever Kaggle answered into "owner/slug". A created
// notebook comes back as "/code/<owner>/<slug>", which is a URL path, not a ref.
func normalizeRef(raw, owner, slug string) string {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "/")
	raw = strings.TrimPrefix(raw, "code/")
	raw = strings.TrimSuffix(raw, "/edit")
	parts := strings.Split(raw, "/")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return raw
	}
	return owner + "/" + slug
}

// CreateNotebook creates a private notebook with the given title and returns
// it. Kaggle derives the page from the slug we send, so the title is slugified.
func (c *Client) CreateNotebook(title string) (Kernel, error) {
	title = strings.TrimSpace(title)
	if len([]rune(title)) < 5 {
		return Kernel{}, fmt.Errorf("a notebook title needs at least 5 characters")
	}
	slug := Slugify(title)
	if slug == "" {
		return Kernel{}, fmt.Errorf("a notebook title needs letters or digits")
	}

	req := map[string]any{
		"slug":                c.Creds.Username + "/" + slug,
		"newTitle":            title,
		"text":                string(notebook.Blank("python")),
		"language":            "python",
		"kernelType":          "notebook",
		"isPrivate":           true,
		"enableInternet":      true,
		"kernelExecutionType": "QUICK_SAVE",
	}

	var out struct {
		Ref           string `json:"ref"`
		URL           string `json:"url"`
		VersionNumber int    `json:"versionNumber"`
		KernelID      int    `json:"kernelId"`
		Error         string `json:"error"`
	}
	if err := c.call("SaveKernel", req, &out); err != nil {
		return Kernel{}, err
	}
	if out.Error != "" {
		return Kernel{}, fmt.Errorf("%s", out.Error)
	}

	ref := normalizeRef(out.Ref, c.Creds.Username, slug)
	k := Kernel{
		ID:       out.KernelID,
		Ref:      ref,
		Title:    title,
		Slug:     slug,
		Owner:    c.Creds.Username,
		Private:  true,
		Internet: true,
		Language: "python",
		Type:     "notebook",
		Version:  out.VersionNumber,
	}
	return k, nil
}
