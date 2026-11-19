package ui

import (
	"strings"

	"github.com/himanshusardana/jupy/internal/kaggle"
)

// listColumns lays out the notebook table's inner area: a marker gutter, a
// flexible title, then the notebook ref and the last-run date pinned right.
func (a *app) listColumns(x, width int) (gutter, titleW, slugX, slugW, dateW, dateX int) {
	gutter = 2
	dateW = 10
	slugW = clamp(width/5, 14, 30)
	dateX = x + width - dateW
	slugX = max(dateX-1-slugW, x+gutter)
	titleW = max(slugX-x-gutter-2, 12)
	return gutter, titleW, slugX, slugW, dateW, dateX
}

func (a *app) drawList(l layout) {
	boxW, boxH := l.w-1, l.h-2
	a.panel(0, 0, boxW, boxH, "", false)

	innerX, innerW := 1, boxW-2
	gutter, titleW, slugX, slugW, dateW, dateX := a.listColumns(innerX, innerW)

	a.put(1, innerX+gutter, titleW, "title", a.th.dim, false)
	a.put(1, slugX, slugW, "notebook", a.th.dim, false)
	a.put(1, dateX, dateW, "updated", a.th.dim, false)
	a.midline(2, 0, boxW, a.th.border)

	if len(a.nbs) == 0 {
		a.put(l.listTop, innerX+1, innerW-1, "no notebooks yet, press a to create one", a.th.dim, false)
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
			a.put(l.listTop+i, innerX, innerW, "", a.th.cursorLine, true)
		}

		a.put(l.listTop+i, innerX, gutter, "  ", titleAttr, false)
		a.put(l.listTop+i, innerX+gutter, titleW, k.Title, titleAttr, false)
		a.put(l.listTop+i, slugX, slugW, k.Slug, metaAttr, false)
		if t := k.LastRunTime(); !t.IsZero() {
			a.put(l.listTop+i, dateX, dateW, t.Format("2006-01-02"), metaAttr, false)
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
