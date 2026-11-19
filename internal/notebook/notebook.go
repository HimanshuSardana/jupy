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

// NormalizeForPush prepares a notebook document for the Kaggle push API: code
// cells are sent without outputs, and each cell's source is joined into one
// string, both of which the server expects. Everything else in the document is
// preserved — dropping nbformat or metadata makes Kaggle's notebook editor fail
// to load the version ("can't access property orig_nbformat").
func NormalizeForPush(doc []byte) ([]byte, error) {
	var parsed map[string]any
	if err := json.Unmarshal(doc, &parsed); err != nil {
		return nil, err
	}
	if err := validateNotebook(parsed); err != nil {
		return nil, err
	}

	cells, _ := parsed["cells"].([]any)
	for _, raw := range cells {
		cell, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := cell["cell_type"].(string); kind == "code" {
			cell["outputs"] = []any{}
		}
		if lines, ok := cell["source"].([]any); ok {
			var b strings.Builder
			for _, line := range lines {
				if text, ok := line.(string); ok {
					b.WriteString(text)
				}
			}
			cell["source"] = b.String()
		}
	}
	return json.Marshal(parsed)
}

// validateNotebook rejects documents the Kaggle renderer cannot open.
func validateNotebook(doc map[string]any) error {
	var missing []string
	if _, ok := doc["cells"].([]any); !ok {
		missing = append(missing, "cells")
	}
	if _, ok := doc["metadata"].(map[string]any); !ok {
		missing = append(missing, "metadata")
	}
	if _, ok := doc["nbformat"]; !ok {
		missing = append(missing, "nbformat")
	}
	if _, ok := doc["nbformat_minor"]; !ok {
		missing = append(missing, "nbformat_minor")
	}
	if len(missing) > 0 {
		return fmt.Errorf("refusing to push an incomplete notebook: missing %s", strings.Join(missing, ", "))
	}
	return nil
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

// InsertCell inserts an empty cell of the given type at index at and keeps the
// source document in sync so a save writes it back.
func (nb *Notebook) InsertCell(at int, kind string) *Cell {
	if at < 0 || at > len(nb.Cells) {
		at = len(nb.Cells)
	}
	nb.Cells = append(nb.Cells, Cell{})
	copy(nb.Cells[at+1:], nb.Cells[at:])
	nb.Cells[at] = Cell{Type: kind, Edited: true}

	if nb.doc != nil {
		cells, _ := nb.doc["cells"].([]any)
		raw := map[string]any{
			"cell_type": kind,
			"metadata":  map[string]any{},
			"source":    []string{},
		}
		if kind == "code" {
			raw["outputs"] = []any{}
			raw["execution_count"] = nil
		}
		cells = append(cells, nil)
		copy(cells[at+1:], cells[at:])
		cells[at] = raw
		nb.doc["cells"] = cells
	}
	return &nb.Cells[at]
}

// DeleteCell removes the cell at index at.
func (nb *Notebook) DeleteCell(at int) bool {
	if at < 0 || at >= len(nb.Cells) {
		return false
	}
	nb.Cells = append(nb.Cells[:at], nb.Cells[at+1:]...)
	if nb.doc != nil {
		if cells, ok := nb.doc["cells"].([]any); ok && at < len(cells) {
			cells = append(cells[:at], cells[at+1:]...)
			nb.doc["cells"] = cells
		}
	}
	return true
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
