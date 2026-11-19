package ui

import (
	"fmt"
	"strings"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshu/jupy/internal/kaggle"
	"github.com/himanshu/jupy/internal/notebook"
)

// Config is everything the browser needs: the notebook list, and how to fetch
// a notebook's cells on demand.
type Config struct {
	User      string
	Notebooks []kaggle.Kernel
	Open      func(kaggle.Kernel) (*notebook.Notebook, error)
	More      func() ([]kaggle.Kernel, error)
}

type focus int

const (
	focusNotebooks focus = iota
	focusCells
)

type app struct {
	scr *goncurses.Window
	cfg Config

	nbs  []kaggle.Kernel
	nsel int
	noff int

	nb    *notebook.Notebook
	csel  int
	coff  int
	pvOff int

	where  focus
	status string
	th     theme
}

func Run(cfg Config) error {
	if cfg.Open == nil {
		return fmt.Errorf("ui: Config.Open is required")
	}

	scr, err := initScreen()
	if err != nil {
		return err
	}
	a := &app{scr: scr, cfg: cfg, nbs: cfg.Notebooks, status: helpLine}
	a.th = newTheme()
	defer goncurses.End()

	return a.loop()
}

func initScreen() (*goncurses.Window, error) {
	scr, err := goncurses.Init()
	if err != nil {
		return nil, err
	}
	goncurses.Cursor(0)
	goncurses.Echo(false)
	goncurses.Raw(true)
	scr.Keypad(true)
	return scr, nil
}

const (
	helpLine = "j/k move  tab pane  enter open  e edit in nvim  r reload  n more  w save  q quit"
	gutter   = 2
)

func (a *app) loop() error {
	for {
		a.draw()
		ch := a.scr.GetChar()
		if ch == 0 {
			continue
		}
		if quit := a.handle(ch); quit {
			return nil
		}
	}
}

func (a *app) handle(ch goncurses.Key) bool {
	switch ch {
	case 'q', 3:
		return true
	case 'j', goncurses.KEY_DOWN:
		a.move(1)
	case 'k', goncurses.KEY_UP:
		a.move(-1)
	case 'g', goncurses.KEY_HOME:
		a.jump(0)
	case 'G', goncurses.KEY_END:
		a.jump(-1)
	case 'J', goncurses.KEY_PAGEDOWN:
		a.previewScroll(1)
	case 'K', goncurses.KEY_PAGEUP:
		a.previewScroll(-1)
	case '\t', 'l', goncurses.KEY_RIGHT:
		a.where = focusCells
	case goncurses.KEY_BTAB, 'h', goncurses.KEY_LEFT:
		a.where = focusNotebooks
	case '\r', '\n', goncurses.KEY_ENTER:
		a.enter()
	case 'e':
		a.editSelectedCell()
	case 'r':
		a.reload()
	case 'n':
		a.more()
	case 'w':
		a.save()
	}
	return false
}

func (a *app) enter() {
	switch a.where {
	case focusNotebooks:
		a.openSelectedNotebook()
	case focusCells:
		a.editSelectedCell()
	}
}

func (a *app) move(delta int) {
	if a.where == focusNotebooks {
		a.nsel = clampIndex(a.nsel+delta, len(a.nbs))
	} else {
		a.csel = clampIndex(a.csel+delta, a.cellCount())
		a.pvOff = 0
	}
}

func (a *app) jump(to int) {
	if a.where == focusNotebooks {
		a.nsel = jumpIndex(to, len(a.nbs))
	} else {
		a.csel = jumpIndex(to, a.cellCount())
		a.pvOff = 0
	}
}

func (a *app) previewScroll(delta int) {
	if a.nb == nil || a.csel >= len(a.nb.Cells) {
		return
	}
	lines := a.previewLines()
	_, _, _, _, prevRows := a.layout()
	maxOff := len(lines) - prevRows
	if maxOff < 0 {
		maxOff = 0
	}
	a.pvOff = clamp(a.pvOff+delta, 0, maxOff)
}

func (a *app) cellCount() int {
	if a.nb == nil {
		return 0
	}
	return len(a.nb.Cells)
}

func (a *app) openSelectedNotebook() {
	if len(a.nbs) == 0 {
		return
	}
	k := a.nbs[a.nsel]
	a.status = "loading " + k.Ref + " ..."
	a.draw()

	nb, err := a.cfg.Open(k)
	if err != nil {
		a.status = err.Error()
		return
	}
	if nb.Title == "" {
		nb.Title = k.Title
	}
	a.nb = nb
	if nb.Language != "" {
		a.nbs[a.nsel].Language = nb.Language
	}
	a.csel, a.coff, a.pvOff = 0, 0, 0
	a.where = focusCells
	a.status = fmt.Sprintf("%s - %d cells", nb.Title, len(nb.Cells))
}

func (a *app) reload() {
	if len(a.nbs) == 0 {
		return
	}
	if a.nb != nil && a.nb.Dirty() > 0 {
		a.status = "not reloaded: unsaved cell edits (w to save a copy)"
		return
	}
	a.openSelectedNotebook()
}

func (a *app) more() {
	if a.cfg.More == nil {
		a.status = "no more pages"
		return
	}
	a.status = "loading more ..."
	a.draw()

	nbs, err := a.cfg.More()
	if err != nil {
		a.status = err.Error()
		return
	}
	if len(nbs) == 0 {
		a.status = "no more notebooks"
		return
	}
	a.nbs = append(a.nbs, nbs...)
	a.status = fmt.Sprintf("%d notebooks", len(a.nbs))
}

func (a *app) save() {
	if a.nb == nil {
		a.status = "nothing open"
		return
	}
	data, err := a.nb.Marshal()
	if err != nil {
		a.status = err.Error()
		return
	}
	name := a.nb.Ref
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == a.nb.Ref && !strings.HasSuffix(name, ".ipynb") {
		name = "notebook"
	}
	if !strings.HasSuffix(name, ".ipynb") {
		name += ".ipynb"
	}
	if err := writeFile(name, data); err != nil {
		a.status = err.Error()
		return
	}
	for i := range a.nb.Cells {
		a.nb.Cells[i].Edited = false
	}
	a.status = "wrote " + name
}

func (a *app) draw() {
	h, w := a.scr.MaxYX()
	a.scr.Erase()

	if h < 12 || w < 46 {
		a.put(0, 0, w, " terminal too small (need 46x12) - press q", a.th.label, false)
		a.scr.Refresh()
		return
	}

	listW, cellsTop, cellsRows, prevTop, prevRows := a.layout()

	a.drawTitle(w)
	a.drawNotebooks(listW, cellsTop, prevRows+cellsRows+1)
	a.drawCells(listW, cellsTop, cellsRows, prevTop, prevRows)
	a.drawStatus(h, w)
	a.scr.Refresh()
}

// layout returns the left pane width, the first cells row, how many rows it
// gets, the preview header row, and the preview height.
func (a *app) layout() (listW, cellsTop, cellsRows, prevTop, prevRows int) {
	h, w := a.scr.MaxYX()
	listW = clamp(w/3, 24, 46)
	bodyBottom := h - 3

	cellsTop = 2
	available := bodyBottom - cellsTop + 1
	cellsRows = clamp(available/2-1, 3, available-2)
	prevTop = cellsTop + cellsRows + 1
	prevRows = bodyBottom - prevTop + 1
	return
}

func (a *app) drawTitle(w int) {
	label := "jupy"
	if a.cfg.User != "" {
		label += "  @" + a.cfg.User
	}
	if a.nb != nil {
		label += "  " + a.nb.Title
		if n := a.nb.Dirty(); n > 0 {
			label += fmt.Sprintf("  [%d edited]", n)
		}
	}
	a.section(0, 0, w, label, true)
}

func (a *app) drawNotebooks(listW, top, rows int) {
	a.section(1, 0, listW, fmt.Sprintf("Notebooks (%d)", len(a.nbs)), a.where == focusNotebooks)

	a.noff = scrollInto(a.nsel, a.noff, rows, len(a.nbs))
	for i := 0; i < rows; i++ {
		idx := a.noff + i
		if idx >= len(a.nbs) {
			break
		}
		k := a.nbs[idx]
		selected := idx == a.nsel && a.where == focusNotebooks
		attr := a.th.normal
		mark := "  "
		if selected {
			attr, mark = a.th.sel, "> "
		}

		row := mark + k.Title
		if b := k.Badges(); b != "" {
			row += "  " + b
		}
		a.put(top+i, 0, listW-11, row, attr, false)
		if t := k.LastRunTime(); !t.IsZero() {
			a.put(top+i, listW-10, 10, t.Format("2006-01-02"), a.th.dim, false)
		}
	}
}

func (a *app) drawCells(listW, top, rows, prevTop, prevRows int) {
	x := listW + gutter
	width := a.scrMaxX() - x

	label := "Cells"
	if a.nb != nil {
		label = fmt.Sprintf("Cells (%d)", len(a.nb.Cells))
	}
	a.section(1, x, width, label, a.where == focusCells)

	if a.nb == nil {
		a.put(top, x, width, "select a notebook and press enter", a.th.dim, false)
		a.section(prevTop, x, width, "Preview", false)
		return
	}

	a.coff = scrollInto(a.csel, a.coff, rows, len(a.nb.Cells))
	for i := 0; i < rows; i++ {
		idx := a.coff + i
		if idx >= len(a.nb.Cells) {
			break
		}
		c := a.nb.Cells[idx]
		selected := idx == a.csel && a.where == focusCells
		attr := a.th.normal
		mark := " "
		if selected {
			attr, mark = a.th.sel, ">"
		}
		edited := " "
		if c.Edited {
			edited = "*"
		}
		row := fmt.Sprintf("%s%s%3d %-3s %s", mark, edited, idx+1, c.Tag(), c.Preview())
		a.put(top+i, x, width, row, attr, false)
	}

	cell := a.nb.Cells[a.csel]
	lines := a.previewLines()
	label = fmt.Sprintf("Cell %d of %d - %s", a.csel+1, len(a.nb.Cells), cell.Type)
	if max := len(lines) - prevRows; max > 0 {
		label += fmt.Sprintf("  [%d%%]", a.pvOff*100/max)
	}
	a.section(prevTop, listW+gutter, prevRows+1, label, true)

	lines = lines[a.pvOff:]
	for i := 0; i < prevRows && i < len(lines); i++ {
		a.put(prevTop+1+i, x, width, lines[i], a.th.normal, false)
	}
}

func (a *app) previewLines() []string {
	if a.nb == nil || a.csel >= len(a.nb.Cells) {
		return nil
	}
	lines := strings.Split(a.nb.Cells[a.csel].Source, "\n")
	for i := range lines {
		lines[i] = sanitize(lines[i])
	}
	return lines
}

func (a *app) drawStatus(h, w int) {
	msg := a.status
	if msg == "" {
		msg = helpLine
	}
	a.put(h-2, 0, w, msg, a.th.normal, false)
	a.put(h-1, 0, w, helpLine, a.th.dim, false)
}

// section draws a label and a hairline rule running to the right edge, the
// card-header treatment used instead of filled panel backgrounds.
func (a *app) section(y, x, width int, label string, focused bool) {
	attr := a.th.labelFor(focused)
	text := " " + label + " "
	a.put(y, x, width, text, attr, false)
	used := len([]rune(sanitize(text)))
	a.rule(y, x+used, width-used, attr)
}

func (a *app) rule(y, x, width int, attr goncurses.Char) {
	h, w := a.scr.MaxYX()
	if width <= 0 || y < 0 || y >= h || x < 0 || x >= w {
		return
	}
	if x+width > w {
		width = w - x
	}
	a.scr.AttrSet(attr)
	for i := 0; i < width; i++ {
		a.scr.MoveAddChar(y, x+i, goncurses.ACS_HLINE)
	}
	a.scr.AttrSet(goncurses.A_NORMAL)
}

func (a *app) scrMaxX() int {
	_, w := a.scr.MaxYX()
	return w
}

func (a *app) put(y, x, width int, s string, attr goncurses.Char, fill bool) {
	if width <= 0 {
		return
	}
	h, w := a.scr.MaxYX()
	if y < 0 || y >= h || x < 0 || x >= w {
		return
	}
	if x+width > w {
		width = w - x
	}
	s = truncate(sanitize(s), width)
	if fill {
		s += strings.Repeat(" ", width-len([]rune(s)))
	}
	a.scr.AttrSet(attr)
	a.scr.MovePrintf(y, x, "%s", s)
	a.scr.AttrSet(goncurses.A_NORMAL)
}

func sanitize(s string) string {
	return strings.NewReplacer("\t", "    ", "\r", " ", "\n", " ").Replace(s)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "~"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	return clamp(i, 0, n-1)
}

func jumpIndex(to, n int) int {
	if n == 0 {
		return 0
	}
	if to < 0 {
		return n - 1
	}
	return clampIndex(to, n)
}

func scrollInto(sel, off, height, n int) int {
	if height <= 0 {
		return 0
	}
	if sel < off {
		off = sel
	}
	if sel >= off+height {
		off = sel - height + 1
	}
	maxOff := n - height
	if maxOff < 0 {
		maxOff = 0
	}
	return clamp(off, 0, maxOff)
}
