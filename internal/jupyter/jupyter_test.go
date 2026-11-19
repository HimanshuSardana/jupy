package jupyter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseBothURLShapes(t *testing.T) {
	const jwt = "eyJhbGciOiJkaXIifQ.payload.sig"

	colab, err := Parse("https://kkb-production.jupyter-proxy.kaggle.net?token=" + jwt)
	if err != nil {
		t.Fatal(err)
	}
	if colab.BaseURL != "https://kkb-production.jupyter-proxy.kaggle.net" || colab.Token != jwt {
		t.Errorf("colab = %+v", colab)
	}

	for _, raw := range []string{
		"https://kkb-production.jupyter-proxy.kaggle.net/k/371531700/" + jwt + "/proxy",
		"https://kkb-production.jupyter-proxy.kaggle.net/k/371531700/" + jwt,
	} {
		vscode, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if vscode.BaseURL != colab.BaseURL || vscode.Token != jwt {
			t.Errorf("vscode = %+v", vscode)
		}
	}
	if colab.Host() != "kkb-production.jupyter-proxy.kaggle.net" {
		t.Errorf("host = %q", colab.Host())
	}
}

func TestParseRejectsJunk(t *testing.T) {
	for _, raw := range []string{"", "  ", "kkb-production.jupyter-proxy.kaggle.net", "ftp://x?token=a", "https://host/api/kernels"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) = nil error", raw)
		}
	}
	if strings.Contains(mustParse(t, "https://h?token=s3cret").String(), "s3cret") {
		t.Error("token must not appear in any user-facing string")
	}
}

func mustParse(t *testing.T, raw string) *Session {
	t.Helper()
	s, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fakeServer(t *testing.T, wsHandler func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	upgrader := websocket.Upgrader{}

	mux.HandleFunc("/api/kernels", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "jwt" {
			http.Error(w, `{"message":"bad token"}`, http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`[{"id":"k1","name":"python3","execution_state":"idle","connections":1}]`))
		case http.MethodPost:
			var body struct{ Name string }
			json.NewDecoder(r.Body).Decode(&body)
			if body.Name != "python3" {
				t.Errorf("start kernel name = %q", body.Name)
			}
			w.Write([]byte(`{"id":"k2","name":"python3","execution_state":"starting"}`))
		}
	})

	mux.HandleFunc("/api/kernels/k1/channels", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_id") == "" || r.URL.Query().Get("token") != "jwt" {
			http.Error(w, "no session", http.StatusForbidden)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		wsHandler(conn)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func send(t *testing.T, conn *websocket.Conn, parent, msgType, channel string, content any) {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	msg := map[string]any{
		"header":        map[string]any{"msg_id": newID(), "msg_type": msgType},
		"parent_header": map[string]any{"msg_id": parent},
		"channel":       channel,
		"content":       json.RawMessage(raw),
	}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteStreamsOutputs(t *testing.T) {
	srv := fakeServer(t, func(conn *websocket.Conn) {
		var req struct {
			Header struct {
				MsgID string `json:"msg_id"`
			} `json:"header"`
			Content struct {
				Code string `json:"code"`
			} `json:"content"`
		}
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if req.Content.Code != "print(1); 2/0" {
			t.Errorf("code = %q", req.Content.Code)
		}
		parent := req.Header.MsgID

		// a message from another client must be ignored
		send(t, conn, "someone-else", "stream", "iopub", map[string]any{"name": "stdout", "text": "nope\n"})
		send(t, conn, parent, "status", "iopub", map[string]any{"execution_state": "busy"})
		send(t, conn, parent, "stream", "iopub", map[string]any{"name": "stdout", "text": "hello\n"})
		send(t, conn, parent, "stream", "iopub", map[string]any{"name": "stderr", "text": "warn\n"})
		send(t, conn, parent, "execute_result", "iopub", map[string]any{
			"execution_count": 7,
			"data":            map[string]any{"text/plain": "42"},
		})
		send(t, conn, parent, "display_data", "iopub", map[string]any{
			"data": map[string]any{"image/png": "iVBORw0K"},
		})
		send(t, conn, parent, "execute_reply", "shell", map[string]any{"status": "error", "execution_count": 7})
		send(t, conn, parent, "error", "iopub", map[string]any{
			"ename":     "ZeroDivisionError",
			"evalue":    "division by zero",
			"traceback": []string{"\x1b[31mZeroDivisionError\x1b[0m: division by zero"},
		})
		send(t, conn, parent, "status", "iopub", map[string]any{"execution_state": "idle"})
		time.Sleep(50 * time.Millisecond)
	})

	s := mustParse(t, srv.URL+"?token=jwt")
	var events []Event
	count, err := s.Execute(context.Background(), "k1", "print(1); 2/0", func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if count != 7 {
		t.Errorf("execution count = %d", count)
	}

	want := []Event{
		{Kind: Stdout, Text: "hello\n"},
		{Kind: Stderr, Text: "warn\n"},
		{Kind: Result, Text: "42"},
		{Kind: Image, Text: "[image/png]"},
		{Kind: Error, Text: "ZeroDivisionError: division by zero"},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %#v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %#v, want %#v", i, events[i], want[i])
		}
	}
}

func TestExecuteReturnsContextError(t *testing.T) {
	srv := fakeServer(t, func(conn *websocket.Conn) {
		var req struct{}
		conn.ReadJSON(&req)
		time.Sleep(500 * time.Millisecond)
	})

	s := mustParse(t, srv.URL+"?token=jwt")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := s.Execute(ctx, "k1", "sleep", func(Event) {})
	if err == nil {
		t.Fatal("want an error when the context expires mid-execution")
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Errorf("Execute did not return promptly: %v", time.Since(start))
	}
}

func TestKernelsAndStartKernel(t *testing.T) {
	srv := fakeServer(t, func(*websocket.Conn) {})
	s := mustParse(t, srv.URL+"?token=jwt")
	ctx := context.Background()

	kernels, err := s.Kernels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(kernels) != 1 || kernels[0].ID != "k1" || kernels[0].State != "idle" {
		t.Errorf("kernels = %+v", kernels)
	}

	k, err := s.StartKernel(ctx, "python3")
	if err != nil {
		t.Fatal(err)
	}
	if k.ID != "k2" {
		t.Errorf("started kernel = %+v", k)
	}
}

func TestBadTokenSurfacesError(t *testing.T) {
	srv := fakeServer(t, func(*websocket.Conn) {})
	s := &Session{BaseURL: srv.URL, Token: "wrong", HTTP: srv.Client()}
	if _, err := s.Kernels(context.Background()); err == nil {
		t.Fatal("want an error for a rejected token")
	} else if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v", err)
	}
}
