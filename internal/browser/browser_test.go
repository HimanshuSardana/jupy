package browser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCommandRespectsBrowserEnv(t *testing.T) {
	t.Setenv("BROWSER", "firefox")
	name, args := command()
	if name != "firefox" || len(args) != 0 {
		t.Errorf("command = %q %v", name, args)
	}
}

func TestCommandDefaultsByOS(t *testing.T) {
	t.Setenv("BROWSER", "")
	name, _ := command()
	if name != "open" && name != "xdg-open" {
		t.Errorf("command = %q", name)
	}
}

func TestOpenRejectsEmptyURL(t *testing.T) {
	if err := Open(""); err == nil {
		t.Fatal("want an error for an empty url")
	}
}

// Open must reject a missing opener instead of silently doing nothing.
func TestOpenMissingOpener(t *testing.T) {
	t.Setenv("BROWSER", "definitely-not-a-browser-binary")
	if err := Open("https://www.kaggle.com/code/me/x"); err == nil {
		t.Fatal("want an error when the opener is missing")
	}
}

// A stub opener records the URL it was given.
func TestOpenStartsOpenerWithURL(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "opened")
	script := filepath.Join(dir, "opener")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$1\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script)

	if err := Open("https://www.kaggle.com/code/me/x"); err != nil {
		t.Fatal(err)
	}

	deadline := 200
	for i := 0; i < deadline; i++ {
		if raw, err := os.ReadFile(out); err == nil {
			if got := string(raw); got != "https://www.kaggle.com/code/me/x\n" {
				t.Fatalf("opener got %q", got)
			}
			return
		}
		wait()
	}
	t.Fatal("opener was not run")
}

func wait() { time.Sleep(10 * time.Millisecond) }
