package ui

import (
	"context"
	"fmt"
	"strings"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/highlight"
	"github.com/himanshusardana/jupy/internal/jupyter"
	"github.com/himanshusardana/jupy/internal/kaggle"
	"github.com/himanshusardana/jupy/internal/notebook"
)

// Config is everything the browser needs: the notebook list, how to fetch a
// notebook's cells, and optionally a Jupyter session to attach to on start.
type Config struct {
	User      string
	Session   string
	Notebooks []kaggle.Kernel
	Open      func(kaggle.Kernel) (*notebook.Notebook, kaggle.Kernel, error)
	More      func() ([]kaggle.Kernel, error)
	Push      func(kaggle.Kernel, []byte) (int, error)

	// StartSession starts an interactive session for a notebook on the given
	// machine shape and returns the Jupyter URLs to try, best first.
	StartSession func(kaggle.Kernel, string) (kaggle.StartedSession, error)
	StopSession  func(id int) error
}

type focus int

const (
	focusNotebooks focus = iota
	focusCells
)

const (
	helpLine  = "j/k move  J/K scroll  tab pane  enter edit  R run  a add  m md  d del  P push  c session  C url  x stop  q quit"
	minWidth  = 60
	minHeight = 14
)

// card is one cell rendered as a box: source, then its output when it has any.
type card struct {
	cell   int
	tag    string
	state  string
	lines  [][]highlight.Span
	outs   [][]highlight.Span
	height int
}

type app struct {
	scr *goncurses.Window
	cfg Config

	nbs  []kaggle.Kernel
	nsel int
	noff int

	nb      *notebook.Notebook
	curMeta kaggle.Kernel
	cards   []card
	csel    int
	ctop    int
	selMove bool

	jup       *session
	sessionID int
	running   int
	runCancel context.CancelFunc
	events    chan execEvent
	outputs   map[int][]jupyter.Event

	where     focus
	status    string
	th        theme
	needsDraw bool

	// ClearOk is forced when content shifts or an external program has
	// scribbled on the terminal, so ncurses never patches a stale model.
	lastCtop, lastNoff int
	forceClear         bool
}

func Run(cfg Config) error {
	if cfg.Open == nil {
		return fmt.Errorf("ui: Config.Open is required")
	}

	scr, err := initScreen()
	if err != nil {
		return err
	}
	a := &app{
		scr:       scr,
		cfg:       cfg,
		nbs:       cfg.Notebooks,
		running:   -1,
		events:    make(chan execEvent, 512),
		outputs:   map[int][]jupyter.Event{},
		needsDraw: true,
	}
	a.th = newTheme()
	defer goncurses.End()

	if cfg.Session != "" {
		a.attach(cfg.Session)
	}

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
	// the tick that lets execution output arrive while the UI waits for keys
	scr.Timeout(120)
	return scr, nil
}

func (a *app) loop() error {
	for {
		a.drainEvents()
		if a.needsDraw {
			a.needsDraw = false
			a.draw()
		}

		ch := a.scr.GetChar()
		if ch <= 0 {
			continue
		}
		if quit := a.handle(ch); quit {
			if a.runCancel != nil {
				a.runCancel()
			}
			return nil
		}
		a.needsDraw = true
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
	case 'J':
		a.scroll(1)
	case 'K':
		a.scroll(-1)
	case goncurses.KEY_PAGEDOWN:
		a.scroll(a.layout().bodyRows / 2)
	case goncurses.KEY_PAGEUP:
		a.scroll(-a.layout().bodyRows / 2)
	case '\t', 'l', goncurses.KEY_RIGHT:
		a.where = focusCells
	case goncurses.KEY_BTAB, 'h', goncurses.KEY_LEFT:
		a.where = focusNotebooks
	case '\r', '\n', goncurses.KEY_ENTER:
		a.enter()
	case 'e':
		a.editSelectedCell()
	case 'R', 18:
		a.runCell(a.csel)
	case 'a':
		a.addCell("code")
	case 'm':
		a.addCell("markdown")
	case 'd':
		a.deleteCell()
	case 'c':
		a.connect()
	case 'C':
		a.connectByURL()
	case 'x':
		a.stopSession()
	case 'r':
		a.reload()
	case 'n':
		a.more()
	case 'w':
		a.save()
	case 'P':
		a.push()
	}
	return false
}

func (a *app) enter() {
	if a.where == focusNotebooks {
		a.openSelectedNotebook()
		return
	}
	a.editSelectedCell()
}

func (a *app) move(delta int) {
	if a.where == focusNotebooks {
		a.nsel = clampIndex(a.nsel+delta, len(a.nbs))
		return
	}
	a.csel = clampIndex(a.csel+delta, len(a.cards))
	a.selMove = true
}

func (a *app) jump(to int) {
	if a.where == focusNotebooks {
		a.nsel = jumpIndex(to, len(a.nbs))
		return
	}
	a.csel = jumpIndex(to, len(a.cards))
	a.selMove = true
}

func (a *app) scroll(delta int) {
	if a.nb == nil || delta == 0 {
		return
	}
	total := 0
	for _, c := range a.cards {
		total += c.height
	}
	max := total - a.layout().bodyRows
	if max < 0 {
		max = 0
	}
	a.ctop = clamp(a.ctop+delta, 0, max)
}

func (a *app) openSelectedNotebook() {
	if len(a.nbs) == 0 {
		return
	}
	if a.running >= 0 && a.runCancel != nil {
		a.runCancel()
		a.running = -1
	}

	k := a.nbs[a.nsel]
	a.status = "loading " + k.Ref + " ..."
	a.draw()

	nb, meta, err := a.cfg.Open(k)
	if err != nil {
		a.status = err.Error()
		return
	}
	a.curMeta = meta
	if nb.Title == "" {
		nb.Title = k.Title
	}
	if nb.Language != "" {
		a.nbs[a.nsel].Language = nb.Language
	}
	a.nb = nb
	a.csel, a.ctop, a.selMove = 0, 0, true
	a.outputs = map[int][]jupyter.Event{}
	for i, c := range nb.Cells {
		if c.Output != "" {
			a.outputs[i] = []jupyter.Event{{Kind: jupyter.Stdout, Text: c.Output}}
		}
	}
	a.buildCards()
	a.where = focusCells
	a.status = fmt.Sprintf("%s - %d cells", nb.Title, len(nb.Cells))
}

func (a *app) buildCards() {
	a.cards = a.cards[:0]
	if a.nb == nil {
		return
	}
	width := a.layout().bodyW - 4
	if width < 8 {
		width = 8
	}
	for i, c := range a.nb.Cells {
		state := ""
		if i == a.running {
			state = "running"
		}
		a.cards = append(a.cards, newCard(i, c, a.kind(c), width, a.outputs[i], state))
	}
}

func (a *app) kind(c notebook.Cell) string {
	switch c.Type {
	case "markdown":
		return "markdown"
	case "code":
		if strings.EqualFold(a.language(), "python") {
			return "python"
		}
	}
	return "text"
}

func (a *app) language() string {
	if a.nb == nil {
		return ""
	}
	return a.nb.Language
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
	// keep output produced in this session in the local copy
	for i := 0; i < len(a.nb.Cells) && len(a.outputs) > 0; i++ {
		if outs, ok := a.outputs[i]; ok && a.nb.Cells[i].Type == "code" {
			a.nb.SetCellOutputs(i, ipynbOutputs(outs))
		}
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

func newCard(index int, c notebook.Cell, kind string, width int, outs []jupyter.Event, state string) card {
	logical := highlight.Lines(c.Source, kind)
	var lines [][]highlight.Span
	for _, line := range logical {
		lines = append(lines, wrapSpans(line, width)...)
	}
	if len(lines) == 0 {
		lines = [][]highlight.Span{{{Style: highlight.Plain}}}
	}

	var outLines [][]highlight.Span
	for _, ev := range outs {
		for _, line := range outputSpans(ev) {
			outLines = append(outLines, wrapSpans(line, width)...)
		}
	}

	height := len(lines) + 2
	if len(outLines) > 0 {
		height += len(outLines) + 1
	}
	return card{cell: index, tag: c.Tag(), state: state, lines: lines, outs: outLines, height: height}
}

func wrapSpans(spans []highlight.Span, width int) [][]highlight.Span {
	if width <= 0 {
		return nil
	}
	var out [][]highlight.Span
	var cur []highlight.Span
	used := 0
	flush := func() {
		out = append(out, cur)
		cur, used = nil, 0
	}

	for _, span := range spans {
		runes := []rune(sanitize(span.Text))
		for len(runes) > 0 {
			room := width - used
			if room <= 0 {
				flush()
				continue
			}
			if len(runes) <= room {
				cur = append(cur, highlight.Span{Text: string(runes), Style: span.Style})
				used += len(runes)
				break
			}
			cur = append(cur, highlight.Span{Text: string(runes[:room]), Style: span.Style})
			runes = runes[room:]
			flush()
		}
	}
	if len(cur) > 0 || len(out) == 0 {
		out = append(out, cur)
	}
	return out
}

type layout struct {
	w, h               int
	boxTop, boxBottom  int
	listX, listW       int
	bodyX, bodyW       int
	innerTop, bodyTop  int
	listRows, bodyRows int
}

func (a *app) layout() layout {
	h, w := a.scr.MaxYX()
	listW := clamp(w/3, 26, 46)
	return layout{
		w: w, h: h,
		boxTop: 1, boxBottom: h - 3,
		listX: 0, listW: listW,
		bodyX: listW + 1, bodyW: max(w-listW-2, 1),
		innerTop: 2, listRows: max(h-5, 1),
		bodyTop: 1, bodyRows: max(h-3, 1),
	}
}

func (a *app) draw() {
	l := a.layout()
	a.scr.Erase()

	if l.h < minHeight || l.w < minWidth {
		a.put(0, 0, l.w, fmt.Sprintf("terminal too small (need %dx%d) - press q", minWidth, minHeight), a.th.warn, false)
		a.scr.Refresh()
		return
	}

	a.noff = scrollInto(a.nsel, a.noff, l.listRows, len(a.nbs))
	if a.nb != nil && len(a.cards) > 0 {
		a.ctop = a.scrollFor(l.bodyRows)
	}
	if a.forceClear || a.ctop != a.lastCtop || a.noff != a.lastNoff {
		a.scr.ClearOk(true)
		a.forceClear = false
		a.lastCtop, a.lastNoff = a.ctop, a.noff
	}

	a.drawLogin(l)
	boxH := l.boxBottom - l.boxTop + 1
	a.panel(l.listX, l.boxTop, l.listW, boxH, fmt.Sprintf("Notebooks (%d)", len(a.nbs)), a.where == focusNotebooks)
	a.drawNotebooks(l)
	a.drawBody(l)
	a.drawStatus(l)
	a.scr.Refresh()
}

func (a *app) drawLogin(l layout) {
	const prefix = "logged in as: "
	name := a.cfg.User
	if name == "" {
		name = "(local files)"
	}
	a.put(0, 1, l.w-2, prefix, a.th.dim, false)
	a.put(0, 1+len(prefix), l.w-2-len(prefix), name, a.th.label, false)
}

// panel draws a box with its label sitting in the top border.
func (a *app) panel(x, y, width, height int, label string, focused bool) {
	if width < 4 || height < 3 {
		return
	}
	border, text := a.th.border, a.th.dim
	if focused {
		border, text = a.th.focusBorder, a.th.label
	}
	right, bottom := x+width-1, y+height-1

	a.hline(y, x, width, border, goncurses.ACS_ULCORNER, goncurses.ACS_URCORNER)
	a.hline(bottom, x, width, border, goncurses.ACS_LLCORNER, goncurses.ACS_LRCORNER)
	a.scr.AttrSet(border)
	for j := 1; j < height-1; j++ {
		a.scr.MoveAddChar(y+j, x, goncurses.ACS_VLINE)
		a.scr.MoveAddChar(y+j, right, goncurses.ACS_VLINE)
	}
	a.scr.AttrSet(goncurses.A_NORMAL)

	if label != "" {
		a.put(y, x+2, width-4, " "+label+" ", text, false)
	}
}

func (a *app) hline(y, x, width int, attr goncurses.Char, left, right goncurses.Char) {
	if width < 2 {
		return
	}
	a.scr.AttrSet(attr)
	a.scr.MoveAddChar(y, x, left)
	for i := 1; i < width-1; i++ {
		a.scr.MoveAddChar(y, x+i, goncurses.ACS_HLINE)
	}
	a.scr.MoveAddChar(y, x+width-1, right)
	a.scr.AttrSet(goncurses.A_NORMAL)
}

func (a *app) midline(y, x, width int, attr goncurses.Char) {
	if width < 2 {
		return
	}
	a.scr.AttrSet(attr)
	a.scr.MoveAddChar(y, x, goncurses.ACS_LTEE)
	for i := 1; i < width-1; i++ {
		a.scr.MoveAddChar(y, x+i, goncurses.ACS_HLINE)
	}
	a.scr.MoveAddChar(y, x+width-1, goncurses.ACS_RTEE)
	a.scr.AttrSet(goncurses.A_NORMAL)
}

func (a *app) drawNotebooks(l layout) {
	x, width, top, rows := l.listX+1, l.listW-2, l.innerTop, l.listRows

	for i := 0; i < rows; i++ {
		idx := a.noff + i
		if idx >= len(a.nbs) {
			break
		}
		k := a.nbs[idx]
		attr, mark := a.th.plain, "  "
		if idx == a.nsel && a.where == focusNotebooks {
			attr, mark = a.th.sel, "> "
		}

		a.put(top+i, x, width-11, mark+k.Title, attr, false)
		if t := k.LastRunTime(); !t.IsZero() {
			a.put(top+i, x+width-10, 10, t.Format("2006-01-02"), a.th.dim, false)
		}
	}
}

func (a *app) drawBody(l layout) {
	x, width, top, rows := l.bodyX, l.bodyW, l.bodyTop, l.bodyRows

	if a.nb == nil {
		a.put(top, x+1, width-1, "select a notebook and press enter", a.th.dim, false)
		return
	}
	if len(a.cards) == 0 {
		a.put(top, x+1, width-1, "this notebook has no cells", a.th.dim, false)
		return
	}

	y := top - a.ctop
	for _, c := range a.cards {
		if y > top+rows {
			break
		}
		a.drawCard(x, y, width, c, c.cell == a.csel && a.where == focusCells, top, top+rows-1)
		y += c.height
	}
}

// scrollFor keeps the selected card in view unless the user scrolled it away.
func (a *app) scrollFor(rows int) int {
	start, height := 0, 0
	for i, c := range a.cards {
		if i == a.csel {
			height = c.height
			break
		}
		start += c.height
	}

	if a.selMove {
		a.selMove = false
		if height <= rows {
			if a.ctop > start {
				a.ctop = start
			}
			if a.ctop+rows < start+height {
				a.ctop = start + height - rows
			}
		} else {
			a.ctop = start
		}
	} else {
		if start+height <= a.ctop {
			a.ctop = start
		}
		if start >= a.ctop+rows {
			a.ctop = start
		}
	}

	total := 0
	for _, c := range a.cards {
		total += c.height
	}
	return clamp(a.ctop, 0, max(total-rows, 0))
}

func (a *app) drawCard(x, y, width int, c card, selected bool, clipTop, clipBottom int) {
	border, label := a.th.border, a.th.dim
	if selected {
		border, label = a.th.focusBorder, a.th.label
	}
	bottom := y + c.height - 1

	if y >= clipTop && y <= clipBottom {
		a.hline(y, x, width, border, goncurses.ACS_ULCORNER, goncurses.ACS_URCORNER)
		title := fmt.Sprintf(" %d %s ", c.cell+1, c.tag)
		if c.state != "" {
			title = fmt.Sprintf(" %d %s - %s ", c.cell+1, c.tag, c.state)
		}
		a.put(y, x+2, width-4, title, label, false)
	}
	if bottom >= clipTop && bottom <= clipBottom {
		a.hline(bottom, x, width, border, goncurses.ACS_LLCORNER, goncurses.ACS_LRCORNER)
	}

	row := y + 1
	for _, spans := range c.lines {
		a.cardRow(row, x, width, border, spans, clipTop, clipBottom)
		row++
	}
	if len(c.outs) > 0 {
		if row >= clipTop && row <= clipBottom {
			a.midline(row, x, width, border)
		}
		row++
		for _, spans := range c.outs {
			a.cardRow(row, x, width, border, spans, clipTop, clipBottom)
			row++
		}
	}
}

func (a *app) cardRow(row, x, width int, border goncurses.Char, spans []highlight.Span, clipTop, clipBottom int) {
	if row < clipTop || row > clipBottom {
		return
	}
	a.scr.AttrSet(border)
	a.scr.MoveAddChar(row, x, goncurses.ACS_VLINE)
	a.scr.MoveAddChar(row, x+width-1, goncurses.ACS_VLINE)
	a.scr.AttrSet(goncurses.A_NORMAL)
	a.putSpans(row, x+2, width-4, spans)
}

func (a *app) putSpans(y, x, width int, spans []highlight.Span) {
	cx := x
	for _, span := range spans {
		room := width - (cx - x)
		if room <= 0 {
			break
		}
		text := truncate(sanitize(span.Text), room)
		if text == "" {
			continue
		}
		a.scr.AttrSet(a.th.style(span.Style))
		a.scr.MovePrintf(y, cx, "%s", text)
		cx += len([]rune(text))
	}
	a.scr.AttrSet(goncurses.A_NORMAL)
}

func (a *app) drawStatus(l layout) {
	right := ""
	if a.jup != nil {
		right = "connected: " + a.jup.label()
	}
	msgWidth := l.w - 2
	if right != "" && len(right)+2 < msgWidth {
		a.put(l.h-2, l.w-2-len(right), l.w-2, right, a.th.dim, false)
		msgWidth = l.w - 4 - len(right)
	}
	msg := a.status
	if msg == "" && a.nb != nil && a.nb.Dirty() > 0 {
		msg = fmt.Sprintf("%d cells edited - P pushes a version, w writes a local copy", a.nb.Dirty())
	}
	if msg != "" {
		a.put(l.h-2, 1, msgWidth, msg, a.th.plain, false)
	}
	a.put(l.h-1, 1, l.w-2, helpLine, a.th.dim, false)
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

// sanitize flattens control characters and folds non-ASCII runes: goncurses
// builds against narrow ncurses, where a multi-byte rune would occupy several
// cells and break every column calculation after it.
func sanitize(s string) string {
	s = strings.NewReplacer("\t", "    ", "\r", " ", "\n", " ").Replace(s)
	if isASCII(s) {
		return s
	}
	runes := []rune(s)
	for i, r := range runes {
		if r > 0x7e || r < 0x20 {
			runes[i] = '?'
		}
	}
	return string(runes)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7e || (s[i] < 0x20 && s[i] != 0) {
			return false
		}
	}
	return true
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

var _ = context.CancelFunc(nil)
