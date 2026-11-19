package notebook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSampleNotebook(t *testing.T) {
	nb, err := Load(filepath.Join("testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	if nb.Language != "python" {
		t.Errorf("language = %q, want python", nb.Language)
	}
	if len(nb.Cells) != 4 {
		t.Fatalf("cells = %d, want 4", len(nb.Cells))
	}

	want := []struct{ kind, tag, preview string }{
		{"markdown", "md", "# Titanic survival"},
		{"code", "py", "import pandas as pd"},
		{"code", "py", "df = pd.read_csv(\"/kaggle/input/titanic/train.csv\")"},
		{"markdown", "md", "## Notes"},
	}
	for i, w := range want {
		c := nb.Cells[i]
		if c.Type != w.kind || c.Tag() != w.tag || c.Preview() != w.preview {
			t.Errorf("cell %d = (%q,%q,%q), want (%q,%q,%q)", i, c.Type, c.Tag(), c.Preview(), w.kind, w.tag, w.preview)
		}
	}
	if got := nb.Cells[1].Source; got != "import pandas as pd\nimport numpy as np\n" {
		t.Errorf("cell 1 source = %q", got)
	}
}

func TestParseAcceptsStringSource(t *testing.T) {
	doc := `{"cells":[{"cell_type":"code","source":"print(1)"}],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	nb, err := Parse("t", "t", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if got := nb.Cells[0].Source; got != "print(1)" {
		t.Errorf("source = %q", got)
	}
	if nb.Language != "python" {
		t.Errorf("language = %q, want python default", nb.Language)
	}
}

func TestParseRejectsNonNotebook(t *testing.T) {
	if _, err := Parse("t", "t", []byte(`{"error":"nope"}`)); err == nil {
		t.Fatal("want error for document without cells")
	}
	if _, err := Parse("t", "t", []byte(`not json`)); err == nil {
		t.Fatal("want error for invalid json")
	}
}

func TestMarshalRoundTripKeepsUnknownFields(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	nb, err := Parse("t", "t", raw)
	if err != nil {
		t.Fatal(err)
	}
	nb.Cells[1].Source = "import polars as pl\n"
	nb.Cells[1].Edited = true
	if nb.Dirty() != 1 {
		t.Fatalf("dirty = %d, want 1", nb.Dirty())
	}

	out, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cells []struct {
			CellType       string   `json:"cell_type"`
			Source         []string `json:"source"`
			Outputs        []any    `json:"outputs"`
			ExecutionCount *int     `json:"execution_count"`
		} `json:"cells"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if got := doc.Cells[1].Source; len(got) != 1 || got[0] != "import polars as pl\n" {
		t.Errorf("saved source = %q", got)
	}
	if len(doc.Metadata) == 0 {
		t.Error("metadata dropped")
	}
	if doc.Cells[2].Outputs == nil || doc.Cells[2].ExecutionCount == nil {
		t.Error("code cell outputs/execution_count dropped")
	}
}

func TestParseOutputs(t *testing.T) {
	doc := `{"cells":[
	 {"cell_type":"code","source":"print(1)",
	  "outputs":[
	    {"output_type":"stream","name":"stdout","text":["1\n","2\n"]},
	    {"output_type":"execute_result","data":{"text/plain":["42"]},"metadata":{}},
	    {"output_type":"display_data","data":{"image/png":"iVBORw0K"}},
	    {"output_type":"error","ename":"ValueError","evalue":"bad","traceback":["\u001b[31mValueError\u001b[0m: bad"]}
	  ]},
	 {"cell_type":"markdown","source":"# t"}
	],"metadata":{},"nbformat":4,"nbformat_minor":5}`

	nb, err := Parse("t", "t", []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n2\n42\n[image/png output]\nValueError: bad\nValueError: bad"
	if got := nb.Cells[0].Output; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if nb.Cells[1].Output != "" {
		t.Errorf("markdown output = %q", nb.Cells[1].Output)
	}
}

func TestInsertAndDeleteCell(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	nb, err := Parse("t", "t", raw)
	if err != nil {
		t.Fatal(err)
	}

	nb.InsertCell(1, "code")
	if len(nb.Cells) != 5 {
		t.Fatalf("cells = %d, want 5", len(nb.Cells))
	}
	if nb.Cells[1].Type != "code" || !nb.Cells[1].Edited {
		t.Errorf("inserted cell = %+v", nb.Cells[1])
	}
	if nb.Cells[0].Type != "markdown" || nb.Cells[2].Type != "code" {
		t.Errorf("insert shifted neighbours: %q %q", nb.Cells[0].Type, nb.Cells[2].Type)
	}

	nb.Cells[1].Source = "print('new')\n"
	out, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cells []struct {
			CellType string   `json:"cell_type"`
			Source   []string `json:"source"`
			Outputs  []any    `json:"outputs"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 5 {
		t.Fatalf("saved cells = %d, want 5", len(doc.Cells))
	}
	if doc.Cells[1].CellType != "code" || len(doc.Cells[1].Source) != 1 || doc.Cells[1].Source[0] != "print('new')\n" {
		t.Errorf("saved inserted cell = %+v", doc.Cells[1])
	}
	if doc.Cells[1].Outputs == nil {
		t.Error("inserted code cell should carry an empty outputs array")
	}

	if ok := nb.DeleteCell(1); !ok {
		t.Fatal("DeleteCell returned false")
	}
	if len(nb.Cells) != 4 || nb.Cells[1].Type != "code" {
		t.Errorf("after delete: %d cells, cell 1 = %q", len(nb.Cells), nb.Cells[1].Type)
	}
	out, err = nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells) != 4 {
		t.Errorf("saved cells after delete = %d, want 4", len(doc.Cells))
	}

	if nb.DeleteCell(99) {
		t.Error("DeleteCell out of range should return false")
	}
}

func TestNormalizeForPushKeepsTopLevelKeys(t *testing.T) {
	doc := `{"cells":[{"cell_type":"code","source":["x = 1"],"outputs":[]}],
	 "metadata":{"kernelspec":{"name":"python3","language":"python"}},"nbformat":4,"nbformat_minor":5}`
	out, err := NormalizeForPush([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"cells", "metadata", "nbformat", "nbformat_minor"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("push document lost %q: %v", key, parsed)
		}
	}
	if meta, _ := parsed["metadata"].(map[string]any); len(meta) == 0 {
		t.Error("metadata was emptied")
	}
	if parsed["nbformat"] != float64(4) {
		t.Errorf("nbformat = %v", parsed["nbformat"])
	}
}

func TestNormalizeForPushRejectsIncompleteNotebook(t *testing.T) {
	for _, doc := range []string{
		`{"cells":[]}`,
		`{"metadata":{},"nbformat":4,"nbformat_minor":5}`,
		`{"cells":[],"metadata":{}}`,
	} {
		if _, err := NormalizeForPush([]byte(doc)); err == nil {
			t.Errorf("NormalizeForPush(%s) = nil error", doc)
		}
	}
}

func TestNormalizeForPush(t *testing.T) {
	doc := `{"cells":[
	 {"cell_type":"code","source":["print(1)\n","print(2)"],"outputs":[{"output_type":"stream","text":"1\n"}]},
	 {"cell_type":"markdown","source":["# hi\n","there"]}
	],"metadata":{},"nbformat":4,"nbformat_minor":5}`
	out, err := NormalizeForPush([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Cells []struct {
			CellType string `json:"cell_type"`
			Source   any    `json:"source"`
			Outputs  []any  `json:"outputs"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Cells[0].Source; got != "print(1)\nprint(2)" {
		t.Errorf("code source = %#v, want a single joined string", got)
	}
	if len(parsed.Cells[0].Outputs) != 0 {
		t.Errorf("outputs must be stripped, got %v", parsed.Cells[0].Outputs)
	}
	if got := parsed.Cells[1].Source; got != "# hi\nthere" {
		t.Errorf("markdown source = %#v", got)
	}
}

// The push path is Parse -> edit -> Marshal -> NormalizeForPush; this is the
// exact chain that lost nbformat/metadata once, so assert the keys survive it.
func TestMarshalThenNormalizeKeepsDocumentShape(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	nb, err := Parse("t", "t", raw)
	if err != nil {
		t.Fatal(err)
	}
	nb.InsertCell(0, "code")
	nb.Cells[0].Source = "print('hi')\n"

	doc, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	pushed, err := NormalizeForPush(doc)
	if err != nil {
		t.Fatalf("NormalizeForPush: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(pushed, &parsed); err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed["metadata"].(map[string]any); !ok {
		t.Errorf("metadata missing after the push chain: %v", parsed)
	}
	if parsed["nbformat"] == nil || parsed["nbformat_minor"] == nil {
		t.Errorf("nbformat fields missing: %v", parsed)
	}
	cells, _ := parsed["cells"].([]any)
	if len(cells) != 5 {
		t.Fatalf("cells = %d, want 5", len(cells))
	}
	first, _ := cells[0].(map[string]any)
	if first["source"] != "print('hi')\n" {
		t.Errorf("source = %#v", first["source"])
	}
	if outs, _ := first["outputs"].([]any); len(outs) != 0 {
		t.Errorf("outputs should be stripped, got %v", outs)
	}
}

func TestSetCellOutputsSurvivesMarshal(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.ipynb"))
	if err != nil {
		t.Fatal(err)
	}
	nb, err := Parse("t", "t", raw)
	if err != nil {
		t.Fatal(err)
	}
	nb.SetCellOutputs(1, []any{map[string]any{
		"output_type": "stream", "name": "stdout", "text": []string{"hi\n"},
	}})

	out, err := nb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cells []struct {
			CellType string `json:"cell_type"`
			Outputs  []struct {
				OutputType string   `json:"output_type"`
				Text       []string `json:"text"`
			} `json:"outputs"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cells[1].Outputs) != 1 || doc.Cells[1].Outputs[0].Text[0] != "hi\n" {
		t.Errorf("cell 1 outputs = %+v", doc.Cells[1].Outputs)
	}
	if len(doc.Cells[2].Outputs) != 1 {
		t.Errorf("untouched cell 2 lost its original output: %d", len(doc.Cells[2].Outputs))
	}
}
