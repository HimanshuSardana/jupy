package ui

import goncurses "github.com/rthornton128/goncurses"

// theme is the accent set for the rule-under-label treatment: labels carry the
// colour and a hairline rule, content sits on the terminal's own background.
type theme struct {
	label  goncurses.Char
	rule   goncurses.Char
	sel    goncurses.Char
	dim    goncurses.Char
	normal goncurses.Char
}

func newTheme() theme {
	t := theme{
		label:  goncurses.A_BOLD,
		rule:   goncurses.A_NORMAL,
		sel:    goncurses.A_BOLD,
		dim:    goncurses.A_DIM,
		normal: goncurses.A_NORMAL,
	}

	if !goncurses.HasColors() || goncurses.StartColor() != nil {
		return t
	}
	goncurses.UseDefaultColors()
	if goncurses.InitPair(1, goncurses.C_MAGENTA, -1) != nil ||
		goncurses.InitPair(2, goncurses.C_CYAN, -1) != nil {
		return t
	}

	accent := goncurses.ColorPair(1)
	pick := goncurses.ColorPair(2)
	t.label |= accent
	t.rule |= accent
	t.sel |= pick
	return t
}

func (t theme) labelFor(focused bool) goncurses.Char {
	if focused {
		return t.label
	}
	return t.dim
}
