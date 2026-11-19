package ui

import (
	goncurses "github.com/rthornton128/goncurses"

	"github.com/himanshusardana/jupy/internal/highlight"
)

type theme struct {
	plain       goncurses.Char
	dim         goncurses.Char
	label       goncurses.Char
	sel         goncurses.Char
	border      goncurses.Char
	focusBorder goncurses.Char
	warn        goncurses.Char

	keyword   goncurses.Char
	str       goncurses.Char
	comment   goncurses.Char
	number    goncurses.Char
	name      goncurses.Char
	decorator goncurses.Char
	heading   goncurses.Char
	emphasis  goncurses.Char
	code      goncurses.Char
	bullet    goncurses.Char

	stderr goncurses.Char
	err    goncurses.Char
	result goncurses.Char
}

func newTheme() theme {
	t := theme{
		plain:       goncurses.A_NORMAL,
		dim:         goncurses.A_DIM,
		label:       goncurses.A_BOLD,
		sel:         goncurses.A_BOLD,
		border:      goncurses.A_DIM,
		focusBorder: goncurses.A_NORMAL,
		warn:        goncurses.A_BOLD,

		keyword:   goncurses.A_BOLD,
		str:       goncurses.A_NORMAL,
		comment:   goncurses.A_DIM,
		number:    goncurses.A_NORMAL,
		name:      goncurses.A_BOLD,
		decorator: goncurses.A_NORMAL,
		heading:   goncurses.A_BOLD,
		emphasis:  goncurses.A_BOLD,
		code:      goncurses.A_NORMAL,
		bullet:    goncurses.A_NORMAL,

		stderr: goncurses.A_NORMAL,
		err:    goncurses.A_BOLD,
		result: goncurses.A_NORMAL,
	}

	if !goncurses.HasColors() || goncurses.StartColor() != nil {
		return t
	}
	goncurses.UseDefaultColors()
	ok := goncurses.InitPair(1, goncurses.C_MAGENTA, -1) == nil &&
		goncurses.InitPair(2, goncurses.C_CYAN, -1) == nil &&
		goncurses.InitPair(3, goncurses.C_GREEN, -1) == nil &&
		goncurses.InitPair(4, goncurses.C_YELLOW, -1) == nil &&
		goncurses.InitPair(5, goncurses.C_BLUE, -1) == nil &&
		goncurses.InitPair(6, goncurses.C_RED, -1) == nil
	if !ok {
		return t
	}

	magenta := goncurses.ColorPair(1)
	cyan := goncurses.ColorPair(2)
	green := goncurses.ColorPair(3)
	yellow := goncurses.ColorPair(4)
	blue := goncurses.ColorPair(5)
	red := goncurses.ColorPair(6)

	t.label |= magenta
	t.heading |= magenta
	t.decorator |= magenta

	t.sel |= cyan
	t.focusBorder |= cyan
	t.name |= cyan
	t.bullet |= cyan

	t.str |= green
	t.code |= green

	t.number |= yellow
	t.warn |= yellow

	t.keyword |= blue

	t.stderr |= yellow
	t.result |= green
	t.err |= red
	return t
}

func (t theme) labelFor(focused bool) goncurses.Char {
	if focused {
		return t.label
	}
	return t.dim
}

func (t theme) style(s highlight.Style) goncurses.Char {
	switch s {
	case highlight.Keyword:
		return t.keyword
	case highlight.String:
		return t.str
	case highlight.Comment:
		return t.comment
	case highlight.Number:
		return t.number
	case highlight.Name:
		return t.name
	case highlight.Decorator:
		return t.decorator
	case highlight.Heading:
		return t.heading
	case highlight.Emphasis:
		return t.emphasis
	case highlight.Code:
		return t.code
	case highlight.Bullet:
		return t.bullet
	case highlight.Stderr:
		return t.stderr
	case highlight.Error:
		return t.err
	case highlight.Result:
		return t.result
	}
	return t.plain
}
