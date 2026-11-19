package ui

import (
	"fmt"
	"strings"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/kaggle"
)

// listColumns lays out the notebook table: a marker gutter, the title, the
// notebook ref and the last-run date.
func (a *app) listColumns(w int) (gutter, titleW, slugW, dateW int) {
	gutter = 2
	dateW = 10
	slugW = clamp(w/4, 14, 30)
	titleW = w - gutter - slugW - dateW - 3
	if titleW < 12 {
		titleW = 12
		slugW = max(w-gutter-titleW-dateW-3, 8)
	}
	return gutter, titleW, slugW, dateW
}

func (a *app) drawList(l layout) {
	gutter, titleW, slugW, dateW := a.listColumns(l.w)

	a.put(0, 1, l.w-2, fmt.Sprintf("[1] notebooks (%d)", len(a.nbs)), a.th.label, false)
	sort := "sort: last run"
	a.put(0, l.w-1-len(sort), len(sort), sort, a.th.dim, false)

	header := strings.Repeat(" ", gutter) + pad("title", titleW) + " " + pad("notebook", slugW) + " " + pad("updated", dateW)
	a.put(1, 0, l.w, header, a.th.dim, false)
	a.ruleRow(2, l.w, a.th.border)

	// group header, like a table's section row
	a.scr.AttrSet(a.th.dim)
	a.scr.MoveAddChar(3, 0, goncurses.ACS_DARROW)
	a.scr.AttrSet(goncurses.A_NORMAL)
	a.put(3, gutter, l.w-gutter, fmt.Sprintf("my notebooks (%d)", len(a.nbs)), a.th.dim, false)

	if len(a.nbs) == 0 {
		a.put(l.listTop, gutter, l.w-gutter, "no notebooks yet - press a to create one", a.th.dim, false)
		return
	}

	for i := 0; i < l.listRows; i++ {
		idx := a.noff + i
		if idx >= len(a.nbs) {
			break
		}
		k := a.nbs[idx]
		attr, mark := a.th.plain, "  "
		if idx == a.nsel {
			attr, mark = a.th.sel, "> "
		}

		row := mark + pad(k.Title, titleW) + " " + pad(k.Slug, slugW) + " "
		if t := k.LastRunTime(); !t.IsZero() {
			row += pad(t.Format("2006-01-02"), dateW)
		}
		a.put(l.listTop+i, 0, l.w, row, attr, false)
	}
}

// newNotebook asks for a title, creates the notebook on Kaggle and opens it.
func (a *app) newNotebook() {
	if a.cfg.CreateNotebook == nil {
		a.status = "creating notebooks needs kaggle credentials"
		return
	}
	title, ok := a.prompt(" new notebook title: ")
	if !ok || strings.TrimSpace(title) == "" {
		a.status = "new notebook cancelled"
		return
	}

	a.status = "creating " + title + " ..."
	a.draw()

	k, err := a.cfg.CreateNotebook(title)
	if err != nil {
		a.status = "could not create " + title + ": " + err.Error()
		return
	}

	a.nbs = append([]kaggle.Kernel{k}, a.nbs...)
	a.nsel, a.noff = 0, 0
	a.openSelectedNotebook()
	if a.nb != nil {
		a.status = "created " + k.Ref + " - a adds a cell, P pushes"
	}
}
