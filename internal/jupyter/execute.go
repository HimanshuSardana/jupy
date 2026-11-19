package jupyter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Kind int

const (
	Stdout Kind = iota
	Stderr
	Result
	Error
	Image
)

type Event struct {
	Kind Kind
	Text string
}

type message struct {
	Header struct {
		MsgID   string `json:"msg_id"`
		MsgType string `json:"msg_type"`
	} `json:"header"`
	ParentHeader struct {
		MsgID string `json:"msg_id"`
	} `json:"parent_header"`
	Channel string          `json:"channel"`
	MsgType string          `json:"msg_type"`
	Content json.RawMessage `json:"content"`
}

func (m message) kind() string {
	if m.MsgType != "" {
		return m.MsgType
	}
	return m.Header.MsgType
}

// Execute runs code on the kernel and streams output events until the kernel
// reports idle. It returns the kernel's execution count.
func (s *Session) Execute(ctx context.Context, kernelID, code string, emit func(Event)) (int, error) {
	sessionID := newID()
	msgID := newID()

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, s.channelsURL(kernelID, sessionID), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return 0, fmt.Errorf("kernel channel: %w", err)
	}
	defer conn.Close()

	request := map[string]any{
		"header": map[string]any{
			"msg_id":   msgID,
			"username": "jupy",
			"session":  sessionID,
			"msg_type": "execute_request",
			"version":  "5.3",
			"date":     time.Now().UTC().Format(time.RFC3339),
		},
		"parent_header": map[string]any{},
		"metadata":      map[string]any{},
		"content": map[string]any{
			"code":             code,
			"silent":           false,
			"store_history":    true,
			"user_expressions": map[string]any{},
			"allow_stdin":      false,
			"stop_on_error":    true,
		},
		"channel": "shell",
	}
	if err := conn.WriteJSON(request); err != nil {
		return 0, fmt.Errorf("send execute_request: %w", err)
	}

	msgs := make(chan message, 64)
	errs := make(chan error, 1)
	go func() {
		for {
			var m message
			if err := conn.ReadJSON(&m); err != nil {
				errs <- err
				return
			}
			select {
			case msgs <- m:
			case <-ctx.Done():
				return
			}
		}
	}()

	count := 0
	failed := false
	replyFailed := false
	for {
		select {
		case <-ctx.Done():
			return count, ctx.Err()

		case err := <-errs:
			return count, fmt.Errorf("kernel channel: %w", err)

		case m := <-msgs:
			if m.ParentHeader.MsgID != msgID {
				continue
			}
			switch m.kind() {
			case "stream":
				var c struct {
					Name string `json:"name"`
					Text string `json:"text"`
				}
				json.Unmarshal(m.Content, &c)
				kind := Stdout
				if c.Name == "stderr" {
					kind = Stderr
				}
				emit(Event{Kind: kind, Text: c.Text})

			case "execute_result", "display_data", "update_display_data":
				var c struct {
					Data           map[string]any `json:"data"`
					ExecutionCount int            `json:"execution_count"`
				}
				json.Unmarshal(m.Content, &c)
				if c.ExecutionCount > 0 {
					count = c.ExecutionCount
				}
				text, _ := c.Data["text/plain"].(string)
				if text == "" {
					if _, ok := c.Data["image/png"]; ok {
						emit(Event{Kind: Image, Text: "[image/png]"})
					}
					continue
				}
				emit(Event{Kind: Result, Text: text})

			case "error":
				var c struct {
					Ename     string   `json:"ename"`
					Evalue    string   `json:"evalue"`
					Traceback []string `json:"traceback"`
				}
				json.Unmarshal(m.Content, &c)
				failed = true
				emit(Event{Kind: Error, Text: joinTraceback(c.Ename, c.Evalue, c.Traceback)})

			case "execute_reply":
				var c struct {
					Status         string `json:"status"`
					ExecutionCount int    `json:"execution_count"`
				}
				json.Unmarshal(m.Content, &c)
				if c.ExecutionCount > 0 {
					count = c.ExecutionCount
				}
				// the traceback arrives on iopub, usually after this reply
				if c.Status == "error" {
					replyFailed = true
				}

			case "status":
				var c struct {
					ExecutionState string `json:"execution_state"`
				}
				json.Unmarshal(m.Content, &c)
				if c.ExecutionState == "idle" {
					if replyFailed && !failed {
						emit(Event{Kind: Error, Text: "execution failed"})
					}
					return count, nil
				}
			}
		}
	}
}

func (s *Session) channelsURL(kernelID, sessionID string) string {
	base := s.BaseURL
	if u, err := url.Parse(base); err == nil {
		switch u.Scheme {
		case "https":
			u.Scheme = "wss"
		case "http":
			u.Scheme = "ws"
		}
		base = u.String()
	}
	return fmt.Sprintf("%s/api/kernels/%s/channels?session_id=%s&token=%s",
		base, url.PathEscape(kernelID), url.QueryEscape(sessionID), url.QueryEscape(s.Token))
}

func joinTraceback(name, value string, traceback []string) string {
	var cleaned []string
	for _, line := range traceback {
		if line = stripANSI(line); line != "" {
			cleaned = append(cleaned, line)
		}
	}
	if len(cleaned) > 0 {
		return strings.Join(cleaned, "\n")
	}
	if name == "" && value == "" {
		return "execution failed"
	}
	return strings.TrimSpace(name + ": " + value)
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
