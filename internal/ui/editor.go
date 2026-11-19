package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/notebook"
)

func editorCommand() string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "nvim"
}

func (a *app) editSelectedCell() {
	if a.nb == nil || a.csel >= len(a.nb.Cells) {
		a.status = "no cell selected"
		return
	}
	cell := &a.nb.Cells[a.csel]
	source, err := a.runEditor(cell)
	if err != nil {
		a.status = err.Error()
		return
	}
	// editors add a trailing newline on save; that alone is not an edit
	if strings.TrimRight(source, "\n") == strings.TrimRight(cell.Source, "\n") {
		a.status = fmt.Sprintf("cell %d unchanged", a.csel+1)
		return
	}
	cell.Source = source
	cell.Edited = true
	a.buildCards()
	a.status = fmt.Sprintf("cell %d updated in the buffer - w writes a .ipynb copy", a.csel+1)
}

// runEditor hands the terminal to the editor with the cell source, plus its
// output in a vertical split when there is any, and reads the source back.
func (a *app) runEditor(cell *notebook.Cell) (string, error) {
	dir, err := os.MkdirTemp("", "jupy-cell-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	codePath := filepath.Join(dir, "cell"+cellExt(*cell, a.nb.Language))
	if err := os.WriteFile(codePath, []byte(cell.Source), 0o600); err != nil {
		return "", err
	}

	files := []string{codePath}
	if cell.Output != "" {
		outPath := filepath.Join(dir, "output.txt")
		if err := os.WriteFile(outPath, []byte(cell.Output+"\n"), 0o600); err != nil {
			return "", err
		}
		files = append(files, outPath)
	}

	argv := strings.Fields(editorCommand())
	if len(argv) == 0 {
		return "", fmt.Errorf("no editor configured: set EDITOR")
	}
	args := append([]string{}, argv[1:]...)
	if len(files) > 1 {
		args = append(args, "-O")
	}
	args = append(args, files...)

	cmd := exec.Command(argv[0], args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

	goncurses.End()
	runErr := cmd.Run()

	scr, initErr := initScreen()
	if initErr != nil {
		return "", initErr
	}
	a.scr = scr
	a.th = newTheme()

	out, readErr := os.ReadFile(codePath)
	if readErr != nil {
		return "", readErr
	}
	if runErr != nil {
		return string(out), fmt.Errorf("editor: %w", runErr)
	}
	return string(out), nil
}

func cellExt(c notebook.Cell, language string) string {
	switch c.Type {
	case "markdown":
		return ".md"
	case "raw":
		return ".txt"
	}
	switch strings.ToLower(language) {
	case "r":
		return ".R"
	case "julia":
		return ".jl"
	case "sql":
		return ".sql"
	}
	return ".py"
}
