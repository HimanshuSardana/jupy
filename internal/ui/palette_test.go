package ui

import "testing"

func TestFilterCommands(t *testing.T) {
	cmds := []command{
		{name: "notebooks"},
		{name: "open notebook"},
		{name: "new notebook"},
		{name: "push notebook"},
		{name: "quit"},
	}

	if got := filterCommands(cmds, ""); len(got) != len(cmds) {
		t.Errorf("empty query = %d commands, want all %d", len(got), len(cmds))
	}
	if got := filterCommands(cmds, "NOTE"); len(got) != 4 {
		t.Errorf("NOTE = %d commands, want 4", len(got))
	}
	if got := filterCommands(cmds, "new"); len(got) != 1 || got[0].name != "new notebook" {
		t.Errorf("new = %v", got)
	}
	if got := filterCommands(cmds, "  quit  "); len(got) != 1 || got[0].name != "quit" {
		t.Errorf("padded quit = %v", got)
	}
	if got := filterCommands(cmds, "zzz"); len(got) != 0 {
		t.Errorf("zzz = %v", got)
	}
}

func TestCommandsPerScreen(t *testing.T) {
	a := &app{}
	a.view = viewList
	listNames := names(a.commands())
	for _, want := range []string{"open notebook", "new notebook", "quit", "connect session"} {
		if !contains(listNames, want) {
			t.Errorf("list commands missing %q: %v", want, listNames)
		}
	}
	if contains(listNames, "run cell") {
		t.Errorf("cell commands should not appear on the table: %v", listNames)
	}

	a.view = viewDetail
	detailNames := names(a.commands())
	for _, want := range []string{"run cell", "add code cell", "push notebook", "save local copy"} {
		if !contains(detailNames, want) {
			t.Errorf("detail commands missing %q: %v", want, detailNames)
		}
	}
}

func names(cmds []command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.name
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
