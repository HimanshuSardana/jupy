package kaggle

import (
	"encoding/json"
	"testing"
)

func TestDoesPushKeepOutputs(t *testing.T) {
	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	c := New(creds)
	ref := "himanshusardana2/notebook175287c276"
	owner, slug := splitRef(ref)

	meta, source, err := c.GetKernel(owner, slug)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(source, &doc); err != nil {
		t.Fatal(err)
	}
	cells, _ := doc["cells"].([]any)
	if len(cells) == 0 {
		t.Skip("no cells")
	}
	cell, _ := cells[0].(map[string]any)
	if kind, _ := cell["cell_type"].(string); kind != "code" {
		cell["cell_type"] = "code"
	}
	cell["outputs"] = []any{map[string]any{
		"output_type": "stream",
		"name":        "stdout",
		"text":        []string{"hello from jupy (push output test)\n"},
	}}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	version, err := c.SaveKernel(meta, raw, "QUICK_SAVE")
	if err != nil {
		t.Fatalf("SaveKernel with outputs: %v", err)
	}
	t.Logf("pushed v%d with outputs", version)

	_, after, err := c.GetKernel(owner, slug)
	if err != nil {
		t.Fatal(err)
	}
	var check map[string]any
	json.Unmarshal(after, &check)
	cells, _ = check["cells"].([]any)
	first, _ := cells[0].(map[string]any)
	outs, _ := first["outputs"].([]any)
	t.Logf("after pull: cell 1 type=%v outputs=%d %v", first["cell_type"], len(outs), outs)
	if len(outs) == 0 {
		t.Log("=> Kaggle dropped the outputs")
	} else {
		t.Log("=> Kaggle kept the outputs")
	}
}
