package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The updater and the new app start apart from the core, through
// systemd-run where it works; a program that cannot start is an error, not
// a silent nothing (the app would quit and not come back).
func TestDetachedStartsAProgramOrSaysWhyNot(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	script := filepath.Join(dir, "program")
	write(t, script, "#!/bin/sh\necho \"$1\" > \""+marker+"\"\n")
	if err := detached(dir, script, "x"); err == nil {
		t.Fatal("a file that is not executable started")
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := detached(dir, script, "ran"); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if data, err := os.ReadFile(marker); err == nil && string(data) == "ran\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the program did not run")
		}
	}
}
