package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/highlight"
	"github.com/himanshusardana/jupy/internal/jupyter"
	"github.com/himanshusardana/jupy/internal/notebook"
)

const maxOutputLines = 400

type session struct {
	client *jupyter.Session
	kernel jupyter.Kernel
	gpu    string
}

func (s *session) label() string {
	parts := []string{s.client.Host()}
	if s.gpu != "" {
		parts = append(parts, s.gpu)
	}
	return strings.Join(parts, " - ")
}

type execEvent struct {
	cell  int
	ev    jupyter.Event
	done  bool
	count int
	err   error
}

// connect prompts for a Kaggle Jupyter server URL and attaches to its kernel.
func (a *app) connect() {
	url, ok := a.prompt(" jupyter session url: ")
	if !ok {
		a.status = "connect cancelled"
		return
	}
	if url == "" {
		return
	}
	a.attach(url)
}

// attach connects to a session URL without prompting (also used at startup).
func (a *app) attach(url string) {
	a.status = "connecting ..."
	a.draw()

	sess, err := jupyter.Parse(url)
	if err != nil {
		a.status = err.Error()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	kernels, err := sess.Kernels(ctx)
	if err != nil {
		a.status = err.Error()
		return
	}

	var kernel jupyter.Kernel
	if len(kernels) > 0 {
		kernel = kernels[0]
	} else {
		kernel, err = sess.StartKernel(ctx, "python3")
		if err != nil {
			a.status = err.Error()
			return
		}
	}

	a.jup = &session{client: sess, kernel: kernel}
	a.status = "connected to " + sess.Host()
	a.probeAccelerator()
}

// probeAccelerator asks the kernel what hardware it is running on.
func (a *app) probeAccelerator() {
	if a.jup == nil {
		return
	}
	a.status = "probing accelerator ..."
	a.draw()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	var out []string
	code := "!nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null || echo cpu"
	if _, err := a.jup.client.Execute(ctx, a.jup.kernel.ID, code, func(e jupyter.Event) {
		if e.Kind == jupyter.Stdout || e.Kind == jupyter.Result {
			out = append(out, strings.Split(strings.TrimSpace(e.Text), "\n")...)
		}
	}); err != nil {
		a.status = "connected to " + a.jup.client.Host() + " (accelerator unknown)"
		return
	}

	a.jup.gpu = summarizeGPU(out)
	a.status = "ready - R runs the selected cell"
}

func summarizeGPU(lines []string) string {
	counts := map[string]int{}
	var order []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "cpu" {
			return "cpu"
		}
		name := line
		if i := strings.Index(line, ","); i > 0 {
			name = strings.TrimSpace(line[:i])
		}
		name = strings.TrimPrefix(name, "NVIDIA ")
		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}
	if len(order) == 0 {
		return ""
	}
	if n := counts[order[0]]; n > 1 {
		return fmt.Sprintf("%s x%d", order[0], n)
	}
	return order[0]
}

func (a *app) runCell(idx int) {
	if a.nb == nil || idx < 0 || idx >= len(a.nb.Cells) {
		return
	}
	cell := a.nb.Cells[idx]
	if cell.Type != "code" {
		a.status = fmt.Sprintf("cell %d is %s, only code cells run", idx+1, cell.Type)
		return
	}
	if a.jup == nil {
		a.status = "not connected - press c and paste a session url"
		return
	}
	if a.running >= 0 {
		a.status = fmt.Sprintf("cell %d is still running", a.running+1)
		return
	}

	a.running = idx
	a.outputs[idx] = nil
	a.buildCards()
	a.selMove = true
	a.status = fmt.Sprintf("running cell %d on %s", idx+1, a.jup.label())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	a.runCancel = cancel

	client, kernelID, source := a.jup.client, a.jup.kernel.ID, cell.Source
	go func() {
		count, err := client.Execute(ctx, kernelID, source, func(ev jupyter.Event) {
			select {
			case a.events <- execEvent{cell: idx, ev: ev}:
			default:
			}
		})
		select {
		case a.events <- execEvent{cell: idx, done: true, count: count, err: err}:
		default:
		}
	}()
}

func (a *app) drainEvents() {
	for {
		select {
		case e := <-a.events:
			a.needsDraw = true
			if e.done {
				a.running = -1
				if a.runCancel != nil {
					a.runCancel()
					a.runCancel = nil
				}
				switch {
				case e.err != nil:
					a.status = fmt.Sprintf("cell %d: %v", e.cell+1, e.err)
				case e.count > 0:
					a.status = fmt.Sprintf("cell %d finished (In [%d])", e.cell+1, e.count)
				default:
					a.status = fmt.Sprintf("cell %d finished", e.cell+1)
				}
				a.buildCards()
				continue
			}
			if a.nb == nil || e.cell >= len(a.nb.Cells) {
				continue
			}
			outs := append(a.outputs[e.cell], e.ev)
			if len(outs) > maxOutputLines {
				outs = outs[len(outs)-maxOutputLines:]
			}
			a.outputs[e.cell] = outs
			a.buildCards()
		default:
			return
		}
	}
}

// prompt reads one line at the bottom of the screen, keeping long input (a
// session URL is ~500 chars) scrolled to the caret.
func (a *app) prompt(label string) (string, bool) {
	l := a.layout()
	row := l.h - 1
	width := l.w - 2
	var buf []rune

	goncurses.Cursor(1)
	defer goncurses.Cursor(0)

	for {
		text := label + string(buf)
		r := []rune(text)
		if len(r) > width {
			r = r[len(r)-width:]
		}
		a.put(row, 0, l.w-1, string(r)+" ", a.th.label, false)
		a.scr.Refresh()

		ch := a.scr.GetChar()
		switch {
		case ch == '\r' || ch == '\n' || ch == goncurses.KEY_ENTER:
			return strings.TrimSpace(string(buf)), true
		case ch == 3 || ch == 27:
			return "", false
		case ch == goncurses.KEY_BACKSPACE, ch == 127, ch == 8:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case ch == 21: // ctrl-u
			buf = nil
		case ch >= 32 && ch < 127:
			buf = append(buf, rune(ch))
		case ch > 127:
			// pasted multibyte input arrives rune by rune in raw mode
			if src := goncurses.KeyString(ch); len([]rune(src)) == 1 {
				buf = append(buf, []rune(src)...)
			}
		}
	}
}

// push saves the buffer to Kaggle as a new version without running it.
func (a *app) push() {
	if a.nb == nil {
		a.status = "nothing open"
		return
	}
	if a.cfg.Push == nil {
		a.status = "push needs kaggle credentials"
		return
	}
	if a.curMeta.Slug == "" {
		a.status = "push needs a kaggle notebook, not a local file"
		return
	}

	doc, err := a.nb.Marshal()
	if err != nil {
		a.status = err.Error()
		return
	}
	text, err := notebook.NormalizeForPush(doc)
	if err != nil {
		a.status = err.Error()
		return
	}

	a.status = "pushing " + a.curMeta.Slug + " ..."
	a.draw()

	version, err := a.cfg.Push(a.curMeta, text)
	if err != nil {
		a.status = "push failed: " + err.Error()
		return
	}
	for i := range a.nb.Cells {
		a.nb.Cells[i].Edited = false
	}
	a.curMeta.Version = version
	a.buildCards()
	a.status = fmt.Sprintf("pushed version %d - reload the notebook on kaggle.com", version)
}

func (a *app) addCell(kind string) {
	if a.nb == nil {
		a.status = "open a notebook first"
		return
	}
	at := a.csel + 1
	a.nb.InsertCell(at, kind)
	a.shiftOutputs(at, 1)
	a.csel = at
	a.selMove = true
	a.buildCards()
	a.editSelectedCell()
	a.buildCards()
	a.status = fmt.Sprintf("added %s cell %d", kind, at+1)
	a.needsDraw = true
}

func (a *app) deleteCell() {
	if a.nb == nil || len(a.nb.Cells) == 0 {
		return
	}
	idx := a.csel
	if !a.nb.DeleteCell(idx) {
		return
	}
	a.shiftOutputs(idx, -1)
	a.csel = clampIndex(idx, len(a.nb.Cells))
	a.selMove = true
	a.buildCards()
	a.status = fmt.Sprintf("deleted cell %d", idx+1)
}

// shiftOutputs keeps the output map aligned when cells move.
func (a *app) shiftOutputs(at, delta int) {
	shifted := make(map[int][]jupyter.Event, len(a.outputs))
	for idx, outs := range a.outputs {
		switch {
		case idx >= at:
			shifted[idx+delta] = outs
		default:
			shifted[idx] = outs
		}
	}
	a.outputs = shifted
}

// ipynbOutputs renders live execution output in notebook output format, so a
// local save carries the output that was just produced.
func ipynbOutputs(events []jupyter.Event) []any {
	var out []any
	var streamName string
	var streamText []string

	flushStream := func() {
		if streamName == "" {
			return
		}
		if len(streamText) > 0 {
			out = append(out, map[string]any{
				"output_type": "stream",
				"name":        streamName,
				"text":        streamText,
			})
		}
		streamName, streamText = "", nil
	}

	lines := func(text string) []string {
		parts := strings.SplitAfter(text, "\n")
		if n := len(parts); n > 1 && parts[n-1] == "" {
			parts = parts[:n-1]
		}
		return parts
	}

	for _, ev := range events {
		switch ev.Kind {
		case jupyter.Stdout, jupyter.Stderr:
			name := "stdout"
			if ev.Kind == jupyter.Stderr {
				name = "stderr"
			}
			if streamName != "" && streamName != name {
				flushStream()
			}
			streamName = name
			streamText = append(streamText, lines(ev.Text)...)

		case jupyter.Result:
			flushStream()
			out = append(out, map[string]any{
				"output_type":     "execute_result",
				"execution_count": nil,
				"data":            map[string]any{"text/plain": lines(ev.Text)},
				"metadata":        map[string]any{},
			})

		case jupyter.Image:
			flushStream()
			out = append(out, map[string]any{
				"output_type": "display_data",
				"data":        map[string]any{"text/plain": lines(ev.Text)},
				"metadata":    map[string]any{},
			})

		case jupyter.Error:
			flushStream()
			out = append(out, map[string]any{
				"output_type": "error",
				"ename":       "",
				"evalue":      "",
				"traceback":   lines(ev.Text),
			})
		}
	}
	flushStream()
	return out
}

func outputSpans(ev jupyter.Event) [][]highlight.Span {
	style := highlight.Plain
	switch ev.Kind {
	case jupyter.Stderr:
		style = highlight.Stderr
	case jupyter.Error:
		style = highlight.Error
	case jupyter.Result:
		style = highlight.Result
	case jupyter.Image:
		style = highlight.Code
	}

	var lines [][]highlight.Span
	for _, line := range strings.Split(strings.TrimRight(ev.Text, "\n"), "\n") {
		lines = append(lines, []highlight.Span{{Text: line, Style: style}})
	}
	return lines
}
