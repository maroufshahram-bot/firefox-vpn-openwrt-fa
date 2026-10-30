//go:build !windows

package main

import (
	"os"

	"golang.org/x/term"
)

// showIntro prints intro, in color on a terminal.
func showIntro() {
	intro(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == "")
}
