package kaggle

import (
	"os"
	"testing"

	"github.com/himanshusardana/jupy/internal/notebook"
)

// TestLiveListAndPull hits the real API with the ambient Kaggle credentials.
// Opt in with JUPY_LIVE=1; it talks to api.kaggle.com and reads only.
func TestLiveListAndPull(t *testing.T) {
	if os.Getenv("JUPY_LIVE") == "" {
		t.Skip("set JUPY_LIVE=1 to run against api.kaggle.com")
	}

	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	c := New(creds)

	kernels, next, err := c.ListKernels("", "", 5)
	if err != nil {
		t.Fatalf("ListKernels: %v", err)
	}
	t.Logf("auth=%s kernels=%d next=%q", creds, len(kernels), next)
	if len(kernels) == 0 {
		t.Skip("no notebooks on this account")
	}
	for _, k := range kernels {
		t.Logf("  %-42s %-24s %s", k.Ref, k.Author, k.LastRun)
	}

	first := kernels[0]
	meta, source, err := c.GetKernel(first.Owner, first.Slug)
	if err != nil {
		t.Fatalf("GetKernel(%s/%s): %v", first.Owner, first.Slug, err)
	}
	nb, err := notebook.Parse(meta.Title, meta.Ref, source)
	if err != nil {
		t.Fatalf("parse %s: %v", meta.Ref, err)
	}
	t.Logf("pulled %s: %d cells, language %s, %d bytes", nb.Ref, len(nb.Cells), nb.Language, len(source))
	if len(nb.Cells) == 0 {
		t.Fatal("notebook has no cells")
	}
	for i, cell := range nb.Cells {
		if i > 4 {
			break
		}
		t.Logf("  cell %d %-8s %s", i+1, cell.Tag(), cell.Preview())
	}
}
