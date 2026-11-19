package notebook

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Cell struct {
	Type   string
	Source string
	Output string
	Edited bool
}

type Notebook struct {
	Title    string
	Ref      string
	Language string
	Cells    []Cell

	doc map[string]any
}

// Parse reads an .ipynb document. The original JSON is kept so a local save
// only rewrites cell sources and preserves everything else.
func Parse(title, ref string, src []byte) (*Notebook, error) {
	var doc map[string]any
	if err := json.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("%s: not a notebook: %w", title, err)
	}

	rawCells, _ := doc["cells"].([]any)
	if rawCells == nil {
		return nil, fmt.Errorf("%s: no cells array", title)
	}

	nb := &Notebook{
		Title:    title,
		Ref:      ref,
		Language: language(doc),
		doc:      doc,
	}
	for _, rc := range rawCells {
		cell, _ := rc.(map[string]any)
		kind, _ := cell["cell_type"].(string)
		nb.Cells = append(nb.Cells, Cell{
			Type:   kind,
			Source: sourceText(cell["source"]),
			Output: cellOutput(cell),
		})
	}
	return nb, nil
}

func Load(path string) (*Notebook, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, path, raw)
}

func language(doc map[string]any) string {
	meta, _ := doc["metadata"].(map[string]any)
	if spec, ok := meta["kernelspec"].(map[string]any); ok {
		if lang, ok := spec["language"].(string); ok && lang != "" {
			return lang
		}
	}
	if info, ok := meta["language_info"].(map[string]any); ok {
		if name, ok := info["name"].(string); ok && name != "" {
			return name
		}
	}
	return "python"
}

// cellOutput flattens a cell's outputs into plain text: stream text, text/plain
// representations, and error tracebacks.
func cellOutput(cell map[string]any) string {
	raw, _ := cell["outputs"].([]any)
	var parts []string
	for _, o := range raw {
		out, _ := o.(map[string]any)
		switch kind, _ := out["output_type"].(string); kind {
		case "stream":
			parts = append(parts, textOf(out["text"]))
		case "execute_result", "display_data", "update_display_data":
			data, _ := out["data"].(map[string]any)
			if text := textOf(data["text/plain"]); text != "" {
				parts = append(parts, text)
			} else if _, ok := data["image/png"]; ok {
				parts = append(parts, "[image/png output]")
			}
		case "error":
			name, _ := out["ename"].(string)
			value, _ := out["evalue"].(string)
			parts = append(parts, strings.TrimSpace(name+": "+value))
			if tb := textOf(out["traceback"]); tb != "" {
				parts = append(parts, tb)
			}
		}
	}

	var kept []string
	for _, p := range parts {
		if p = strings.TrimRight(stripANSI(p), "\n"); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "\n")
}

func textOf(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []any:
		var b strings.Builder
		for _, item := range s {
			b.WriteString(textOf(item))
		}
		return b.String()
	}
	return ""
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func sourceText(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []any:
		var b strings.Builder
		for _, line := range s {
			if l, ok := line.(string); ok {
				b.WriteString(l)
			}
		}
		return b.String()
	}
	return ""
}

// Marshal returns the notebook as .ipynb, with edited cell sources written back.
func (nb *Notebook) Marshal() ([]byte, error) {
	if nb.doc == nil {
		return nil, fmt.Errorf("%s: no source document", nb.Title)
	}
	rawCells, _ := nb.doc["cells"].([]any)
	for i, rc := range rawCells {
		if i >= len(nb.Cells) {
			break
		}
		if cell, ok := rc.(map[string]any); ok {
			cell["source"] = splitLines(nb.Cells[i].Source)
		}
	}
	return json.MarshalIndent(nb.doc, "", " ")
}

func splitLines(source string) []string {
	if source == "" {
		return []string{}
	}
	lines := strings.SplitAfter(source, "\n")
	if last := lines[len(lines)-1]; last == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (nb *Notebook) Dirty() int {
	n := 0
	for _, c := range nb.Cells {
		if c.Edited {
			n++
		}
	}
	return n
}

// Tag is the short kind marker shown in the cell list.
func (c Cell) Tag() string {
	switch c.Type {
	case "markdown":
		return "md"
	case "raw":
		return "raw"
	case "code":
		return "py"
	}
	if c.Type == "" {
		return "?"
	}
	return c.Type
}

// Preview is the first non-blank line of the cell, for one-row display.
func (c Cell) Preview() string {
	for _, line := range strings.Split(c.Source, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "(empty)"
}

func (c Cell) Markdown() bool { return c.Type == "markdown" }
