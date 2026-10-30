//go:build linux || darwin

package main

// runPlatform: the init system restarts the service after an update and
// removes it after an uninstall, so nothing is left behind here.
func runPlatform(string, []string) int {
	usage()
	return exitUsage
}
