package ui

import (
	"fmt"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/highlight"
	"github.com/himanshusardana/jupy/internal/jupyter"
	"github.com/himanshusardana/jupy/internal/notebook"
)

func (a *app) drawDetail(l layout) {
	a.put(0, l.colX, l.colW, a.detailTitle(), a.th.label, false)
	back := "esc back"
	a.put(0, l.colX+l.colW-len(back), len(back), back, a.th.dim, false)

	if a.nb == nil {
		a.put(l.bodyTop, l.colX, l.colW, "no notebook open", a.th.dim, false)
		return
	}
	if len(a.cards) == 0 {
		a.put(l.bodyTop, l.colX, l.colW, "this notebook has no cells - a adds one", a.th.dim, false)
		return
	}

	y := l.bodyTop - a.ctop
	for _, c := range a.cards {
		if y > l.bodyTop+l.bodyRows {
			break
		}
		a.drawCard(l.colX, y, l.colW, c, c.cell == a.csel, l.bodyTop, l.bodyTop+l.bodyRows-1)
		y += c.height + cardGap
	}
}

func (a *app) detailTitle() string {
	if a.nb == nil {
		return "[2] notebook"
	}
	title := fmt.Sprintf("[2] %s (%d cells)", a.nb.Ref, len(a.nb.Cells))
	if n := a.nb.Dirty(); n > 0 {
		title += fmt.Sprintf(" - %d edited", n)
	}
	return title
}

// scrollFor keeps the selected card in view unless the user scrolled it away.
func (a *app) scrollFor(rows int) int {
	start, height := 0, 0
	for i, c := range a.cards {
		if i == a.csel {
			height = c.height
			break
		}
		start += c.height + cardGap
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

	total := -(cardGap)
	for _, c := range a.cards {
		total += c.height + cardGap
	}
	return clamp(a.ctop, 0, max(total-rows, 0))
}

func (a *app) drawCard(x, y, width int, c card, selected bool, clipTop, clipBottom int) {
	border, label := a.th.border, a.th.dim
	if selected {
		border, label = a.th.focusBorder, a.th.label
	}
	bottom := y + c.height - 1

	pad := cardPadding
	if y >= clipTop && y <= clipBottom {
		a.hline(y, x, width, border, goncurses.ACS_ULCORNER, goncurses.ACS_URCORNER)
		title := fmt.Sprintf("%d %s", c.cell+1, c.tag)
		if c.state != "" {
			title += " - " + c.state
		}
		a.put(y, x+1+pad, width-2-2*pad, " "+title+" ", label, false)
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
	a.putSpans(row, x+1+cardPadding, width-2-2*cardPadding, spans)
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
