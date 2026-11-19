// Package browser opens a URL in the user's default browser.
package browser

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// command builds the opener command; a variable so tests can observe it.
var command = func() (string, []string) {
	if b := os.Getenv("BROWSER"); b != "" {
		return b, nil
	}
	if runtime.GOOS == "darwin" {
		return "open", nil
	}
	return "xdg-open", nil
}

// Open hands the URL to the desktop opener and returns once it has been
// started; the browser itself keeps running.
func Open(url string) error {
	if url == "" {
		return fmt.Errorf("nothing to open")
	}
	name, args := command()
	bin, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("cannot open a browser: %s not found (set BROWSER)", name)
	}
	cmd := exec.Command(bin, append(args, url)...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot open a browser: %w", err)
	}
	go cmd.Wait()
	return nil
}
