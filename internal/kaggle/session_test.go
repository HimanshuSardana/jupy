package kaggle

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func sessionServer(t *testing.T, ops []string, createStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var called []string
	polls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(raw, &body)
		called = append(called, r.URL.Path)

		switch r.URL.Path {
		case "/v1/kernels.KernelsApiService/CreateKernelSession":
			if createStatus != 0 {
				w.WriteHeader(createStatus)
				w.Write([]byte(`{"error":{"code":409,"message":"session exists"}}`))
				return
			}
			if body["slug"] != "me/kernel" {
				t.Errorf("slug = %v", body["slug"])
			}
			if shape, ok := body["machineShape"]; ok && shape != "NvidiaTeslaT4" {
				t.Errorf("machineShape = %v", shape)
			}
			if internet, ok := body["enableInternet"]; ok && internet != true {
				t.Errorf("enableInternet = %v", internet)
			}
			w.Write([]byte(`{"name":"operations/create-kernel-session/371548759","done":false,"metadata":{"kernelSessionId":371548759}}`))

		case "/v1/common.OperationsService/GetOperation":
			if body["name"] != "operations/create-kernel-session/371548759" {
				t.Errorf("poll name = %v", body["name"])
			}
			reply := ops[min(polls, len(ops)-1)]
			polls++
			w.Write([]byte(reply))

		case "/v1/kernels.KernelsApiService/CancelKernelSession":
			w.Write([]byte(`{}`))

		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &called
}

func TestStartSessionReturnsJupyterURLs(t *testing.T) {
	ops := []string{
		`{"name":"operations/create-kernel-session/371548759","done":false}`,
		`{"name":"operations/create-kernel-session/371548759","done":true,"response":{
			"jupyterUrl":"https://kkb-production.jupyter-proxy.kaggle.net/k/371531700/JWTSEG/proxy",
			"tokenizedJupyterUrl":"https://kkb-production.jupyter-proxy.kaggle.net?token=JWTSEG",
			"jupyterTokenPresent":true
		}}`,
	}
	srv, _ := sessionServer(t, ops, 0)

	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL

	internet := true
	started, err := c.StartSession(context.Background(), SessionRequest{
		Slug: "me/kernel", Language: "python", KernelType: "notebook",
		MachineShape: "NvidiaTeslaT4", Internet: &internet,
		Timeout: 5 * time.Second, Poll: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.SessionID != 371548759 {
		t.Errorf("session id = %d, want the one from the operation metadata", started.SessionID)
	}
	if len(started.URLs) < 2 {
		t.Fatalf("urls = %v", started.URLs)
	}
	if want := "https://kkb-production.jupyter-proxy.kaggle.net/k/371531700/JWTSEG/proxy?token=JWTSEG"; started.URLs[0] != want {
		t.Errorf("first url = %q, want %q", started.URLs[0], want)
	}
	for _, u := range started.URLs {
		if u == "" {
			t.Errorf("empty url in %v", started.URLs)
		}
	}
}

func TestStartSessionRunningSessionIsDistinct(t *testing.T) {
	srv, _ := sessionServer(t, []string{`{"done":true}`}, http.StatusConflict)
	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL

	_, err := c.StartSession(context.Background(), SessionRequest{
		Slug: "me/kernel", Timeout: time.Second, Poll: time.Millisecond,
	})
	if !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("err = %v, want ErrSessionRunning", err)
	}
}

func TestStartSessionWithoutURLIsErrorAndCleansUp(t *testing.T) {
	ops := []string{`{"name":"operations/create-kernel-session/7","done":true,"metadata":{"kernelSessionId":7},"response":{}}`}
	srv, called := sessionServer(t, ops, 0)

	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL

	if _, err := c.StartSession(context.Background(), SessionRequest{
		Slug: "me/kernel", Timeout: 2 * time.Second, Poll: time.Millisecond,
	}); err == nil {
		t.Fatal("want an error when no jupyter url is returned")
	}

	cancelled := false
	for _, path := range *called {
		if path == "/v1/kernels.KernelsApiService/CancelKernelSession" {
			cancelled = true
		}
	}
	if !cancelled {
		t.Errorf("session should be cancelled so it does not burn quota; calls = %v", *called)
	}
}

func TestStartSessionOperationError(t *testing.T) {
	ops := []string{`{"name":"operations/create-kernel-session/9","done":true,"error":{"code":9,"message":"quota exhausted"}}`}
	srv, _ := sessionServer(t, ops, 0)
	c := New(Creds{Username: "me", Key: "k"})
	c.BaseURL = srv.URL

	_, err := c.StartSession(context.Background(), SessionRequest{
		Slug: "me/kernel", Timeout: 2 * time.Second, Poll: time.Millisecond,
	})
	if err == nil || err.Error() == "" {
		t.Fatal("want an error for a failed operation")
	}
	if want := "quota exhausted"; !contains(err.Error(), want) {
		t.Errorf("err = %v, want it to mention %q", err, want)
	}
}

func TestSessionURLsFallback(t *testing.T) {
	raw := json.RawMessage(`{"someRenamedField":{"server":"https://kkb-production.jupyter-proxy.kaggle.net/k/1/ABC/proxy"}}`)
	urls := sessionURLs(raw)
	if len(urls) != 1 || urls[0] != "https://kkb-production.jupyter-proxy.kaggle.net/k/1/ABC/proxy" {
		t.Errorf("urls = %v", urls)
	}

	if got := sessionURLs(json.RawMessage(`{"response":"nothing"}`)); len(got) != 0 {
		t.Errorf("urls = %v, want none", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
