package ui

import goncurses "github.com/rthornton128/goncurses"

type theme struct {
	title  goncurses.Char
	header goncurses.Char
	status goncurses.Char
	sel    goncurses.Char
	dim    goncurses.Char
	normal goncurses.Char
}

func newTheme() theme {
	t := theme{
		title:  goncurses.A_BOLD | goncurses.A_REVERSE,
		header: goncurses.A_BOLD,
		status: goncurses.A_REVERSE,
		sel:    goncurses.A_REVERSE,
		dim:    goncurses.A_DIM,
		normal: goncurses.A_NORMAL,
	}

	if !goncurses.HasColors() || goncurses.StartColor() != nil {
		return t
	}
	goncurses.UseDefaultColors()
	if goncurses.InitPair(1, goncurses.C_CYAN, -1) != nil ||
		goncurses.InitPair(2, goncurses.C_BLACK, goncurses.C_CYAN) != nil {
		return t
	}

	head := goncurses.ColorPair(1)
	pick := goncurses.ColorPair(2)
	t.title |= head
	t.header |= head
	t.status |= head
	t.sel |= pick
	return t
}

func (t theme) headerFor(focused bool) goncurses.Char {
	if focused {
		return t.header
	}
	return t.dim
}

func (t theme) row(selected bool) goncurses.Char {
	if selected {
		return t.sel
	}
	return t.normal
}
