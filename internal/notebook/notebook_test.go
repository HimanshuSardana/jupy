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
