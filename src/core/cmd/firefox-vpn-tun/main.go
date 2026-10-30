//go:generate go run ../../tools/winres -program tun

// Command firefox-vpn-tun is the TUN service of the desktop TUN mode: it
// holds the TUN adapter, its routes, and the DNS filters with the system's
// rights, for the core, which stays an ordinary program (internal/tunservice,
// protocol internal/tunapi). The app carries it beside the core and
// installs it once: on Windows as a service in Program Files, on Linux as
// a systemd service in /opt, on macOS as a launchd daemon in
// /Library/Application Support.
//
//	firefox-vpn-tun install     install or update the service for this user (asks for administrator rights)
//	firefox-vpn-tun uninstall   remove the service (no prompt while it runs)
//	firefox-vpn-tun update      make the running service this signed executable
//	firefox-vpn-tun -version    print the version
//
// On macOS, install and uninstall take -prompt, the reason the
// administrator prompt shows (the app passes it in the user's language).
//
// Started with no command, as from Explorer, it explains what it is and
// links the project (on Windows in a console window of its own).
//
// Exit status: 0 done, 1 failed (the reason on the standard error), 2 bad
// arguments, 3 the administrator permission was not given.
package main

import (
	"fmt"
	"os"

	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// Exit statuses.
const (
	exitFailed    = 1
	exitUsage     = 2
	exitCancelled = 3
)

func main() {
	// Opened by hand (no command): say what this is, and end without an
	// error: Windows Terminal keeps a window whose program failed open and
	// starts it again on Enter.
	if len(os.Args) < 2 {
		showIntro()
		return
	}
	switch os.Args[1] {
	case "-version", "--version", "version":
		fmt.Println(version.Current)
		return
	}
	os.Exit(run(os.Args[1], os.Args[2:]))
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: firefox-vpn-tun install | uninstall | update | -version")
	os.Exit(exitUsage)
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "firefox-vpn-tun:", err)
	return exitFailed
}
