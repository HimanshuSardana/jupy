// Package jupyter talks to a Kaggle Jupyter server: the REST API for kernels
// and the websocket channel that executes cells.
//
// Kaggle mints the interactive session URL in the browser (Run -> Kaggle
// Jupyter Server -> Manually Connect), not through the public API, so the URL
// arrives from the user and its embedded JWT is treated as a secret: it is held
// in memory only and never logged.
package jupyter

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Session struct {
	BaseURL string // scheme://host, no session path
	Token   string // session JWT from the URL
	HTTP    *http.Client
}

// Parse accepts either URL Kaggle shows:
//
//	Colab-compatible:  https://host?token=<jwt>
//	VS Code-compatible: https://host/k/<session>/<jwt>/proxy
func Parse(raw string) (*Session, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty session URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("session URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("session URL: want an http(s) URL")
	}

	token := u.Query().Get("token")
	if token == "" {
		// /k/<session>/<jwt>[/proxy] — the trailing segment varies between
		// the URL shapes Kaggle shows
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 3 && parts[0] == "k" {
			token = parts[2]
		}
	}
	if token == "" {
		return nil, fmt.Errorf("session URL: no token in query or in a /k/<id>/<jwt>/proxy path")
	}

	return &Session{
		BaseURL: u.Scheme + "://" + u.Host,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Host is the part of the session safe to show in the UI.
// String must never leak the session token.
func (s *Session) String() string {
	return "jupyter session at " + s.Host()
}

func (s *Session) Host() string {
	u, err := url.Parse(s.BaseURL)
	if err != nil {
		return s.BaseURL
	}
	return u.Host
}

type Kernel struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	State        string `json:"execution_state"`
	LastActivity string `json:"last_activity"`
	Connections  int    `json:"connections"`
}

func (s *Session) Kernels(ctx context.Context) ([]Kernel, error) {
	var out []Kernel
	if err := s.do(ctx, http.MethodGet, "/api/kernels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// StartKernel starts a kernel, e.g. "python3".
func (s *Session) StartKernel(ctx context.Context, name string) (Kernel, error) {
	var out Kernel
	body := map[string]any{"name": name}
	if err := s.do(ctx, http.MethodPost, "/api/kernels", body, &out); err != nil {
		return Kernel{}, err
	}
	return out, nil
}

func (s *Session) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.BaseURL+path+s.tokenQuery(), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("jupyter %s %s: %s: %s", method, path, resp.Status, truncateBody(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (s *Session) tokenQuery() string {
	return "?token=" + url.QueryEscape(s.Token)
}

func truncateBody(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 200 {
		text = text[:200] + "..."
	}
	return text
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
