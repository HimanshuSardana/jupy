package kaggle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.kaggle.com"

type Client struct {
	BaseURL string
	Creds   Creds
	HTTP    *http.Client
}

func New(creds Creds) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		Creds:   creds,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Kernel is one row of ApiKernelMetadata. ListKernels only fills ref, title,
// author (a display name) and lastRunTime; the rest arrives with GetKernel.
type Kernel struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Slug     string `json:"slug"`
	Language string `json:"language"`
	Type     string `json:"kernelType"`
	Machine  string `json:"machineShape"`
	Private  bool   `json:"isPrivate"`
	GPU      bool   `json:"enableGpu"`
	TPU      bool   `json:"enableTpu"`
	Votes    int    `json:"totalVotes"`
	Version  int    `json:"currentVersionNumber"`
	LastRun  string `json:"lastRunTime"`

	Owner string `json:"-"`
}

// normalize derives owner and slug from ref, which is the only place the API
// puts them in list responses.
func (k *Kernel) normalize() {
	if i := strings.Index(k.Ref, "/"); i > 0 {
		if k.Owner == "" {
			k.Owner = k.Ref[:i]
		}
		if k.Slug == "" {
			k.Slug = k.Ref[i+1:]
		}
	}
	if k.Title == "" {
		k.Title = k.Slug
	}
}

func (k Kernel) LastRunTime() time.Time {
	t, err := time.Parse(time.RFC3339, k.LastRun)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (k Kernel) Badges() string {
	var b []string
	if k.Language != "" {
		b = append(b, k.Language)
	}
	if k.GPU || k.TPU {
		b = append(b, "GPU")
	}
	if k.Private {
		b = append(b, "private")
	}
	return strings.Join(b, " ")
}

type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kaggle: %d %s", e.Code, e.Message)
}

// ListKernels returns one page of the given user's notebooks. An empty user
// means the authenticated user (group PROFILE).
func (c *Client) ListKernels(user, pageToken string, pageSize int) ([]Kernel, string, error) {
	req := map[string]any{
		"sortBy":   "DATE_RUN",
		"pageSize": pageSize,
		"group":    "PROFILE",
	}
	if user != "" {
		req["user"] = user
	}
	if pageToken != "" {
		req["pageToken"] = pageToken
	}

	var out struct {
		Kernels       []Kernel `json:"kernels"`
		NextPageToken string   `json:"nextPageToken"`
	}
	if err := c.call("ListKernels", req, &out); err != nil {
		return nil, "", err
	}
	for i := range out.Kernels {
		out.Kernels[i].normalize()
	}
	return out.Kernels, out.NextPageToken, nil
}

// GetKernel downloads a notebook: metadata plus the blob.source, which for
// kernel type "notebook" is the .ipynb JSON document.
func (c *Client) GetKernel(user, slug string) (Kernel, []byte, error) {
	req := map[string]any{"userName": user, "kernelSlug": slug}

	var out struct {
		Metadata Kernel `json:"metadata"`
		Blob     struct {
			Source string `json:"source"`
		} `json:"blob"`
	}
	if err := c.call("GetKernel", req, &out); err != nil {
		return Kernel{}, nil, err
	}
	out.Metadata.Ref = firstNonEmpty(out.Metadata.Ref, user+"/"+slug)
	out.Metadata.normalize()
	if out.Blob.Source == "" {
		return out.Metadata, nil, fmt.Errorf("%s/%s: empty notebook source", user, slug)
	}
	return out.Metadata, []byte(out.Blob.Source), nil
}

func (c *Client) call(method string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := c.BaseURL + "/v1/kernels.KernelsApiService/" + method
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if c.Creds.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Creds.Token)
	} else {
		req.SetBasicAuth(c.Creds.Username, c.Creds.Key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "jupy-tui/0.1")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return decodeError(resp.StatusCode, data)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func decodeError(status int, data []byte) error {
	var envelope struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &envelope) == nil && envelope.Error.Message != "" {
		return &APIError{Code: envelope.Error.Code, Message: envelope.Error.Message}
	}
	return &APIError{Code: status, Message: strings.TrimSpace(string(data))}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
