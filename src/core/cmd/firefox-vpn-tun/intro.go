package main

import (
	"fmt"
	"io"

	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// projectURL is where people find the app this service belongs to.
const projectURL = "https://github.com/armanlife/firefox-vpn-client"

// intro is what the service says when it is started with no command, as
// someone does who opens it by hand: what it is.
// color adds the terminal's colors.
func intro(out io.Writer, color bool) {
	c := func(code string) string {
		if !color {
			return ""
		}
		return code
	}
	bold, dim, orange, cyan, off := c("\x1b[1m"), c("\x1b[2m"), c("\x1b[38;5;208m"), c("\x1b[36m"), c("\x1b[0m")
	fmt.Fprintf(out, "\n  %s%s◆ Firefox VPN · TUN service%s  %s%s%s\n", bold, orange, off, dim, version.Current, off)
	fmt.Fprintf(out, "  %s──────────────────────────────────────────────────%s\n", dim, off)
	fmt.Fprintf(out, "  %sTUN mode%s service for Firefox VPN.\n", bold, off)
	fmt.Fprintf(out, "\n  %s%s%s\n\n", cyan, projectURL, off)
}
