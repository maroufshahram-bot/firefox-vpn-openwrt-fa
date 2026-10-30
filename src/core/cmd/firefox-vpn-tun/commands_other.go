//go:build !windows && !linux && !darwin

package main

import "errors"

// run runs a command; the TUN service is written for Windows, Linux, and
// macOS.
func run(string, []string) int {
	return fail(errors.New("the TUN mode is not available on this system"))
}
