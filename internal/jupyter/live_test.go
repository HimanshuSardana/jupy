package jupyter

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveSession runs a cell on a real Kaggle Jupyter server. Opt in with
// JUPY_SESSION_URL=<colab- or vscode-compatible URL>.
func TestLiveSession(t *testing.T) {
	raw := os.Getenv("JUPY_SESSION_URL")
	if raw == "" {
		t.Skip("set JUPY_SESSION_URL to a Kaggle Jupyter server URL")
	}

	s, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("session: %s", s)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	kernels, err := s.Kernels(ctx)
	if err != nil {
		t.Fatalf("Kernels: %v", err)
	}
	t.Logf("kernels: %d", len(kernels))
	if len(kernels) == 0 {
		t.Skip("no kernel running in this session")
	}
	for _, k := range kernels {
		t.Logf("  %s %s state=%s connections=%d", k.ID, k.Name, k.State, k.Connections)
	}

	kernel := kernels[0]
	if kernel.State != "idle" {
		t.Logf("kernel is %s, waiting for idle", kernel.State)
		time.Sleep(5 * time.Second)
	}

	events := []Event{}
	count, err := s.Execute(ctx, kernel.ID, "import sys; print('hello from jupy'); print(sys.version.split()[0]); 6*7", func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	t.Logf("execution_count=%d events=%d", count, len(events))
	for _, e := range events {
		t.Logf("  kind=%d %q", e.Kind, e.Text)
	}

	events = nil
	_, err = s.Execute(ctx, kernel.ID, "!nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null || echo 'no gpu'", func(e Event) {
		events = append(events, e)
	})
	if err != nil {
		t.Fatalf("GPU probe: %v", err)
	}
	for _, e := range events {
		t.Logf("  gpu: %q", e.Text)
	}
}
