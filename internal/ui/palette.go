package ui

import (
	"strings"

	goncurses "github.com/rthornton128/goncurses"
)

// command is one palette entry. Commands that need a notebook report that
// themselves, so the list stays the same on both screens.
type command struct {
	name string
	run  func()
}

func (a *app) commands() []command {
	cmds := []command{
		{name: "notebooks", run: a.backToList},
		{name: "open notebook", run: a.openSelectedNotebook},
		{name: "new notebook", run: a.newNotebook},
	}
	if a.view == viewDetail {
		cmds = append(cmds,
			command{name: "run cell", run: func() { a.runCell(a.csel) }},
			command{name: "add code cell", run: func() { a.addCell("code") }},
			command{name: "add markdown cell", run: func() { a.addCell("markdown") }},
			command{name: "delete cell", run: a.deleteCell},
			command{name: "push notebook", run: a.push},
			command{name: "save local copy", run: a.save},
			command{name: "reload notebook", run: a.reload},
		)
	}
	return append(cmds,
		command{name: "connect session", run: a.connect},
		command{name: "paste session url", run: a.connectByURL},
		command{name: "stop session", run: a.stopSession},
		command{name: "open in browser", run: a.openInBrowser},
		command{name: "quit", run: func() { a.quitRequested = true }},
	)
}

// filterCommands keeps the commands whose name contains the query.
func filterCommands(cmds []command, query string) []command {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return cmds
	}
	var out []command
	for _, c := range cmds {
		if strings.Contains(strings.ToLower(c.name), query) {
			out = append(out, c)
		}
	}
	return out
}

// palette is the ctrl-p command palette: filter as you type, move with j/k or
// the arrows, enter runs, esc cancels.
func (a *app) palette() {
	all := a.commands()
	var query []rune
	sel := 0

	for {
		matches := filterCommands(all, string(query))
		sel = clamp(sel, 0, max(len(matches)-1, 0))

		a.draw()
		a.drawPalette(query, matches, sel)

		ch := a.scr.GetChar()
		switch {
		case ch == keyEscape || ch == keyCtrlC:
			a.status = ""
			return
		case ch == '\r' || ch == '\n' || ch == goncurses.KEY_ENTER:
			if len(matches) > 0 {
				a.status = ""
				matches[sel].run()
				return
			}
		case ch == goncurses.KEY_DOWN || ch == keyCtrlN:
			sel = clampIndex(sel+1, len(matches))
		case ch == goncurses.KEY_UP:
			sel = clampIndex(sel-1, len(matches))
		case ch == goncurses.KEY_BACKSPACE, ch == 127, ch == 8:
			if len(query) > 0 {
				query = query[:len(query)-1]
				sel = 0
			} else {
				return
			}
		case ch == keyCtrlU:
			query, sel = nil, 0
		case ch >= 32 && ch < 127:
			query = append(query, rune(ch))
			sel = 0
		}
	}
}

func (a *app) drawPalette(query []rune, matches []command, sel int) {
	l := a.layout()
	width := clamp(l.w-24, 36, 64)
	height := clamp(len(matches), 1, 10) + 3
	x := clamp((l.w-width)/2, 0, max(l.w-width, 0))
	y := clamp(l.h/4, 0, max(l.h-height, 0))

	a.panel(x, y, width, height, "command", true)
	for row := y + 1; row < y+height-1; row++ {
		a.put(row, x+1, width-2, "", a.th.plain, true)
	}

	input := "> " + string(query)
	a.put(y+1, x+2, width-4, input, a.th.label, false)

	if len(matches) == 0 {
		a.put(y+2, x+2, width-4, "no match", a.th.dim, false)
		a.scr.Refresh()
		return
	}
	for i, cmd := range matches {
		if i >= height-3 {
			break
		}
		attr, mark := a.th.plain, "  "
		if i == sel {
			attr, mark = a.th.sel, "> "
		}
		a.put(y+2+i, x+2, width-4, mark+cmd.name, attr, true)
	}
	a.scr.Refresh()
}
