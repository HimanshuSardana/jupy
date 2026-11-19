package kaggle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.kaggle.com"

// Creds are the Kaggle username/API-key pair from kaggle.json. The API accepts
// them as HTTP Basic auth on both the legacy and the RPC endpoints.
type Creds struct {
	Username string
	Key      string
}

// LoadCreds reads KAGGLE_USERNAME/KAGGLE_KEY, then $KAGGLE_CONFIG_DIR/kaggle.json,
// then ~/.kaggle/kaggle.json.
func LoadCreds() (Creds, error) {
	if u, k := os.Getenv("KAGGLE_USERNAME"), os.Getenv("KAGGLE_KEY"); u != "" && k != "" {
		return Creds{Username: u, Key: k}, nil
	}

	dir := os.Getenv("KAGGLE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Creds{}, err
		}
		dir = filepath.Join(home, ".kaggle")
	}
	path := filepath.Join(dir, "kaggle.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		return Creds{}, fmt.Errorf("no Kaggle credentials: write %s (or run `kaggle auth login`), or set KAGGLE_USERNAME/KAGGLE_KEY", path)
	}
	var file struct {
		Username string `json:"username"`
		Key      string `json:"key"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return Creds{}, fmt.Errorf("%s: %w", path, err)
	}
	if file.Username == "" || file.Key == "" {
		return Creds{}, fmt.Errorf("%s: username or key is empty", path)
	}
	return Creds{Username: file.Username, Key: file.Key}, nil
}

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

// Kernel is one row of ApiKernelMetadata (the kernels ListKernels response).
type Kernel struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Slug     string `json:"slug"`
	Language string `json:"language"`
	Type     string `json:"kernelType"`
	Private  bool   `json:"isPrivate"`
	GPU      bool   `json:"enableGpu"`
	TPU      bool   `json:"enableTpu"`
	Votes    int    `json:"totalVotes"`
	Version  int    `json:"currentVersionNumber"`
	LastRun  string `json:"lastRunTime"`
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
	if k.GPU {
		b = append(b, "GPU")
	}
	if k.TPU {
		b = append(b, "TPU")
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
	req.SetBasicAuth(c.Creds.Username, c.Creds.Key)
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
