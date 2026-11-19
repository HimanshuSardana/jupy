package ui

import (
	"fmt"
	"strings"

	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/kaggle"
)

// listColumns lays out the notebook table: a marker gutter, a flexible title,
// then the notebook ref and the last-run date pinned to the right edge.
func (a *app) listColumns(w int) (gutter, titleW, slugX, slugW, dateW int) {
	gutter = 2
	dateW = 10
	slugW = clamp(w/5, 14, 30)
	slugX = max(w-1-dateW-1-slugW, gutter)
	titleW = max(slugX-gutter-2, 12)
	return gutter, titleW, slugX, slugW, dateW
}

func (a *app) drawList(l layout) {
	gutter, titleW, slugX, slugW, dateW := a.listColumns(l.w)

	a.put(0, gutter, titleW, "title", a.th.dim, false)
	a.put(0, slugX, slugW, "notebook", a.th.dim, false)
	a.put(0, l.w-1-dateW, dateW, "updated", a.th.dim, false)
	a.ruleRow(1, l.w, a.th.border)

	// group header, like a table's section row
	a.scr.AttrSet(a.th.dim)
	a.scr.MoveAddChar(2, 0, goncurses.ACS_DARROW)
	a.scr.AttrSet(goncurses.A_NORMAL)
	a.put(2, gutter, l.w-gutter, fmt.Sprintf("my notebooks (%d)", len(a.nbs)), a.th.dim, false)

	if len(a.nbs) == 0 {
		a.put(l.listTop, 0, l.w, "no notebooks yet - press a to create one", a.th.dim, false)
		return
	}

	for i := 0; i < l.listRows; i++ {
		idx := a.noff + i
		if idx >= len(a.nbs) {
			break
		}
		k := a.nbs[idx]
		titleAttr, metaAttr := a.th.plain, a.th.dim
		if idx == a.nsel {
			titleAttr, metaAttr = a.th.cursorLine, a.th.cursorLine
			a.put(l.listTop+i, 0, l.w, "", a.th.cursorLine, true)
		}

		a.put(l.listTop+i, 0, gutter, "  ", titleAttr, false)
		a.put(l.listTop+i, gutter, titleW, k.Title, titleAttr, false)
		a.put(l.listTop+i, slugX, slugW, k.Slug, metaAttr, false)
		if t := k.LastRunTime(); !t.IsZero() {
			a.put(l.listTop+i, l.w-1-dateW, dateW, t.Format("2006-01-02"), metaAttr, false)
		}
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
