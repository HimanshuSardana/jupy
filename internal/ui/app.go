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
// notebook's cells, how to write to Kaggle, and optionally a Jupyter session to
// attach to on start.
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

	// CreateNotebook creates a private notebook and returns it.
	CreateNotebook func(title string) (kaggle.Kernel, error)
}

// view is the screen the app is showing: the notebook table, or one notebook's
// cells.
type view int

const (
	viewList view = iota
	viewDetail
)

const (
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

	view          view
	status        string
	quitRequested bool
	th            theme
	needsDraw     bool

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

const (
	keyEscape = 27
	keyCtrlP  = 16
	keyCtrlN  = 14
	keyCtrlR  = 18
	keyCtrlU  = 21
	keyCtrlC  = 3
)

func (a *app) handle(ch goncurses.Key) bool {
	if ch == 'q' {
		return true
	}
	if ch == keyCtrlP {
		a.palette()
		return a.quitRequested
	}
	if a.view == viewList {
		return a.handleList(ch)
	}
	return a.handleDetail(ch)
}

func (a *app) handleList(ch goncurses.Key) bool {
	switch ch {
	case keyCtrlC:
		return true
	case 'j', goncurses.KEY_DOWN:
		a.nsel = clampIndex(a.nsel+1, len(a.nbs))
	case 'k', goncurses.KEY_UP:
		a.nsel = clampIndex(a.nsel-1, len(a.nbs))
	case 'g', goncurses.KEY_HOME:
		a.nsel = clampIndex(0, len(a.nbs))
	case 'G', goncurses.KEY_END:
		a.nsel = jumpIndex(-1, len(a.nbs))
	case 'n':
		a.more()
	case 'r':
		a.status = fmt.Sprintf("%d notebooks, refreshed on start", len(a.nbs))
	case '\r', '\n', goncurses.KEY_ENTER, 'l', goncurses.KEY_RIGHT:
		a.openSelectedNotebook()
	case 'a':
		a.newNotebook()
	case 'o':
		a.openInBrowser()
	}
	return false
}

func (a *app) handleDetail(ch goncurses.Key) bool {
	switch ch {
	case keyCtrlC:
		return true
	case keyEscape, 'h', goncurses.KEY_LEFT:
		a.backToList()
	case 'j', goncurses.KEY_DOWN:
		a.moveCell(1)
	case 'k', goncurses.KEY_UP:
		a.moveCell(-1)
	case 'g', goncurses.KEY_HOME:
		a.jumpCell(0)
	case 'G', goncurses.KEY_END:
		a.jumpCell(-1)
	case 'J':
		a.scroll(1)
	case 'K':
		a.scroll(-1)
	case goncurses.KEY_PAGEDOWN:
		a.scroll(a.layout().bodyRows / 2)
	case goncurses.KEY_PAGEUP:
		a.scroll(-a.layout().bodyRows / 2)
	case '\r', '\n', goncurses.KEY_ENTER, 'e':
		a.editSelectedCell()
	case 'R', keyCtrlR:
		a.runCell(a.csel)
	case 'a':
		a.addCell("code")
	case 'm':
		a.addCell("markdown")
	case 'd':
		a.deleteCell()
	case 'P':
		a.push()
	case 'w':
		a.save()
	case 'x':
		a.stopSession()
	case 'c':
		a.connect()
	case 'C':
		a.connectByURL()
	case 'o':
		a.openInBrowser()
	case 'r':
		a.reload()
	}
	return false
}

func (a *app) backToList() {
	a.view = viewList
	a.status = ""
	a.ctop = 0
	a.forceClear = true
}

func (a *app) moveCell(delta int) {
	a.csel = clampIndex(a.csel+delta, len(a.cards))
	a.selMove = true
}

func (a *app) jumpCell(to int) {
	a.csel = jumpIndex(to, len(a.cards))
	a.selMove = true
}

func (a *app) scroll(delta int) {
	if a.view != viewDetail || a.nb == nil || delta == 0 {
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
	if nb.Title == "" {
		nb.Title = k.Title
	}
	if nb.Language != "" {
		a.nbs[a.nsel].Language = nb.Language
	}
	a.nb = nb
	a.curMeta = meta
	a.csel, a.ctop, a.selMove = 0, 0, true
	a.outputs = map[int][]jupyter.Event{}
	for i, c := range nb.Cells {
		if c.Output != "" {
			a.outputs[i] = []jupyter.Event{{Kind: jupyter.Stdout, Text: c.Output}}
		}
	}
	a.buildCards()
	a.view = viewDetail
	a.forceClear = true
	a.status = fmt.Sprintf("%s - %d cells", nb.Title, len(nb.Cells))
}

func (a *app) buildCards() {
	a.cards = a.cards[:0]
	if a.nb == nil {
		return
	}
	width := a.layout().contentW
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
	a.applyOutputs()

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

// applyOutputs writes output produced in this session into the document, so
// saving and pushing both carry what the cells printed.
func (a *app) applyOutputs() {
	if a.nb == nil {
		return
	}
	for i := 0; i < len(a.nb.Cells); i++ {
		outs, ok := a.outputs[i]
		if !ok || len(outs) == 0 || a.nb.Cells[i].Type != "code" {
			continue
		}
		a.nb.SetCellOutputs(i, ipynbOutputs(outs))
	}
}

// layout is the screen geometry shared by both views: the cell stream area on
// the detail screen, and the status and help rows.
type layout struct {
	w, h              int
	bodyTop, bodyRows int
	cardX, cardW      int
	contentW          int
	statusY, helpY    int
	listTop, listRows int
}

// Readable measure: cells sit in a centred column, wide enough to read but not
// so wide that lines run across a large screen.
const (
	cardMargin  = 4
	cardMeasure = 108
	cardPadding = 2
	cardGap     = 1
)

func (a *app) layout() layout {
	h, w := a.scr.MaxYX()

	cardW := w - 2*cardMargin
	if cardW > cardMeasure {
		cardW = cardMeasure
	}
	if cardW < 20 {
		cardW = max(w-2, 20)
	}
	if cardW > w-2 {
		cardW = w - 2
	}
	cardX := (w - cardW) / 2

	return layout{
		w: w, h: h,
		bodyTop: 2, bodyRows: max(h-4, 1),
		cardX: cardX, cardW: cardW,
		contentW: cardW - 2 - 2*cardPadding,
		statusY:  h - 2, helpY: h - 1,
		listTop: 3, listRows: max(h-6, 1),
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

	if a.view == viewDetail && a.nb != nil && len(a.cards) > 0 {
		a.ctop = a.scrollFor(l.bodyRows)
	}
	a.noff = scrollInto(a.nsel, a.noff, l.listRows, len(a.nbs))

	if a.forceClear || a.ctop != a.lastCtop || a.noff != a.lastNoff {
		a.scr.ClearOk(true)
		a.forceClear = false
		a.lastCtop, a.lastNoff = a.ctop, a.noff
	}

	if a.view == viewList {
		a.drawList(l)
	} else {
		a.drawDetail(l)
	}
	a.drawStatus(l)
	a.scr.Refresh()
}

func (a *app) drawStatus(l layout) {
	right := ""
	if a.jup != nil {
		right = "connected: " + a.jup.label()
	}
	msgWidth := l.w - 2
	if right != "" && len(right)+2 < msgWidth {
		a.put(l.statusY, l.w-2-len(right), l.w-2, right, a.th.dim, false)
		msgWidth = l.w - 4 - len(right)
	}

	msg := a.status
	if msg == "" && a.view == viewDetail && a.nb != nil && a.nb.Dirty() > 0 {
		msg = fmt.Sprintf("%d cells edited - P pushes a version, w writes a local copy", a.nb.Dirty())
	}
	if msg != "" {
		a.put(l.statusY, 1, msgWidth, msg, a.th.plain, false)
	}
	a.put(l.helpY, 1, l.w-2, helpFor(a.view), a.th.dim, false)
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

// ruleRow draws a full-width hairline.
func (a *app) ruleRow(y, width int, attr goncurses.Char) {
	if width <= 0 {
		return
	}
	a.scr.AttrSet(attr)
	for col := 0; col < width-1; col++ {
		a.scr.MoveAddChar(y, col, goncurses.ACS_HLINE)
	}
	a.scr.AttrSet(goncurses.A_NORMAL)
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

func pad(s string, width int) string {
	r := []rune(sanitize(s))
	if len(r) > width {
		return truncate(s, width)
	}
	return string(r) + strings.Repeat(" ", width-len(r))
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

func helpFor(v view) string {
	if v == viewList {
		return "j/k move  enter open  a new  o browser  n more  ctrl-p commands  q quit"
	}
	return "j/k move  J/K scroll  enter edit  R run  a/m add  d del  P push  esc back  ctrl-p commands  q quit"
}
