package highlight

import (
	"strings"
	"testing"
)

func join(spans []Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func stylesOf(spans []Span) map[string]Style {
	out := map[string]Style{}
	for _, s := range spans {
		out[s.Text] = s.Style
	}
	return out
}

func needsNoHash(spans []Span) bool {
	return join(spans) == "Title **bold**"
}

func TestPythonLineKeepsTextIntact(t *testing.T) {
	src := `def train(model, lr=1e-3):  # comment with 'quote'` + "\n" + `    x = "hi # not a comment"` + "\n" + `    return {"k": 2}`
	lines := Lines(src, "python")
	if len(lines) != 3 {
		t.Fatalf("lines = %d", len(lines))
	}
	for i, want := range strings.Split(src, "\n") {
		if got := join(lines[i]); got != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}

	first := stylesOf(lines[0])
	if first["def"] != Keyword || first["train"] != Name {
		t.Errorf("def/name styles = %v", first)
	}
	if first["# comment with 'quote'"] != Comment {
		t.Errorf("comment style = %v", first)
	}

	second := stylesOf(lines[1])
	if second[`"hi # not a comment"`] != String {
		t.Errorf("string style = %v", second)
	}
	if _, ok := second["# not a comment\""]; ok {
		t.Error("hash inside a string must not start a comment")
	}

	third := stylesOf(lines[2])
	if _, ok := third["2"]; !ok {
		t.Errorf("number token missing: %v", third)
	}
}

func TestPythonTripleQuotedStringSpansLines(t *testing.T) {
	src := "s = \"\"\"first\nsecond\nthird\"\"\"\nx = 1"
	lines := Lines(src, "python")
	if got := join(lines[1]); got != "second" {
		t.Fatalf("middle line = %q", got)
	}
	if lines[1][0].Style != String {
		t.Errorf("middle line style = %v", lines[1][0].Style)
	}
	if got := stylesOf(lines[3]); got["1"] != Number {
		t.Errorf("line after docstring = %v", got)
	}
	if lines[2][0].Style != String {
		t.Errorf("closing line style = %v", lines[2][0].Style)
	}
}

func TestPythonDecoratorAndNumbers(t *testing.T) {
	line := Lines(`@torch.no_grad()`+"\n"+`y = 0xff + 1.5`, "python")
	if line[0][0].Style != Decorator || line[0][0].Text != "@torch.no_grad" {
		t.Errorf("decorator = %+v", line[0][0])
	}
	if got := stylesOf(line[1]); got["0xff"] != Number || got["1.5"] != Number {
		t.Errorf("numbers = %v", got)
	}
}

func TestMarkdown(t *testing.T) {
	src := "# Title **bold**\n\n- item with `code`\n\n```python\nx = 1\n```"
	lines := Lines(src, "markdown")

	if lines[0][0].Style != Heading || needsNoHash(lines[0]) != true {
		t.Errorf("heading = %v", lines[0])
	}
	if len(lines[1]) != 0 {
		t.Errorf("blank line = %v", lines[1])
	}
	bullet := lines[2]
	if bullet[0].Text != "-" || bullet[0].Style != Bullet {
		t.Errorf("bullet = %+v", bullet[0])
	}
	if got := join(bullet); got != "- item with code" {
		t.Errorf("bullet text = %q", got)
	}
	if stylesOf(bullet)["code"] != Code {
		t.Errorf("inline code = %v", stylesOf(bullet))
	}

	if lines[4] != nil {
		t.Errorf("fence marker should not render: %v", lines[4])
	}
	if lines[5][0].Style != Code || lines[5][0].Text != "x = 1" {
		t.Errorf("fenced body = %v", lines[5])
	}
}

func TestMarkdownEmphasisAndLink(t *testing.T) {
	spans := Lines("a **bold** b [docs](https://x.y)", "markdown")[0]
	styles := stylesOf(spans)
	if styles["bold"] != Emphasis {
		t.Errorf("emphasis = %v", styles)
	}
	if got := join(spans); got != "a bold b docs" {
		t.Errorf("text = %q", got)
	}
}

func TestUnknownKindIsPlain(t *testing.T) {
	lines := Lines("SELECT * FROM t -- x", "sql")
	if lines[0][0].Style != Plain {
		t.Errorf("style = %v", lines[0][0].Style)
	}
	if got := join(lines[0]); got != "SELECT * FROM t -- x" {
		t.Errorf("text = %q", got)
	}
}

func TestUnterminatedStringDoesNotHang(t *testing.T) {
	lines := Lines(`x = "unterminated`, "python")
	if got := join(lines[0]); got != `x = "unterminated` {
		t.Errorf("text = %q", got)
	}
}
