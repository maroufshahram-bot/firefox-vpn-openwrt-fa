//go:build !windows && !linux && !darwin

package update

import "time"

// Updates install by themselves on Windows, Linux, and macOS only;
// elsewhere the app offers the download.
const platformSupported = false

func dataDirOK(string, string) string { return "" }

func updating(string) bool { return false }

func lockApp(string) (func(), error) { return nil, errNotSupported }

func waitForExit([]int, time.Duration) error { return errNotSupported }

func startUpdater(string, string) error { return errNotSupported }

func startApp(string, []string) error { return errNotSupported }

func setInstalledVersion(string, string) error { return nil }
