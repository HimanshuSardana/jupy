package kaggle

import (
	"os"
	"testing"
)

// TestLivePushRoundTrip pushes a notebook's own unchanged content as a new
// version and checks the metadata survives. Opt in with
// JUPY_LIVE_PUSH=owner/slug.
func TestLivePushRoundTrip(t *testing.T) {
	ref := os.Getenv("JUPY_LIVE_PUSH")
	if ref == "" {
		t.Skip("set JUPY_LIVE_PUSH=owner/slug to push a version")
	}

	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	c := New(creds)

	owner, slug := splitRef(ref)
	before, source, err := c.GetKernel(owner, slug)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: id=%d v=%d private=%v gpu=%v tpu=%v net=%v shape=%q docker=%q",
		before.ID, before.Version, before.Private, before.GPU, before.TPU, before.Internet,
		before.Machine, before.Docker)
	t.Logf("sources: datasets=%v comps=%v kernels=%v models=%v tags=%v",
		before.Datasets, before.Comps, before.Kernels, before.Models, before.Categories)

	text, err := normalizeForLive(source)
	if err != nil {
		t.Fatal(err)
	}

	version, err := c.SaveKernel(before, text, "QUICK_SAVE")
	if err != nil {
		t.Fatalf("SaveKernel: %v", err)
	}
	t.Logf("pushed version %d", version)

	after, _, err := c.GetKernel(owner, slug)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after:  id=%d v=%d private=%v gpu=%v tpu=%v net=%v shape=%q docker=%q",
		after.ID, after.Version, after.Private, after.GPU, after.TPU, after.Internet,
		after.Machine, after.Docker)
	t.Logf("sources: datasets=%v comps=%v kernels=%v models=%v tags=%v",
		after.Datasets, after.Comps, after.Kernels, after.Models, after.Categories)

	if after.Version <= before.Version {
		t.Errorf("version did not advance: %d -> %d", before.Version, after.Version)
	}
	if after.Private != before.Private || after.GPU != before.GPU || after.TPU != before.TPU || after.Internet != before.Internet {
		t.Errorf("flags changed: before %+v after %+v", before, after)
	}
	if after.Machine != before.Machine {
		t.Errorf("machine shape changed: %q -> %q", before.Machine, after.Machine)
	}
	if len(before.Datasets) != len(after.Datasets) || len(before.Comps) != len(after.Comps) {
		t.Errorf("sources changed: before %v/%v after %v/%v", before.Datasets, before.Comps, after.Datasets, after.Comps)
	}
}

func splitRef(ref string) (string, string) {
	for i := 0; i < len(ref); i++ {
		if ref[i] == '/' {
			return ref[:i], ref[i+1:]
		}
	}
	return "", ref
}
