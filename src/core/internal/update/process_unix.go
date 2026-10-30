//go:build linux || darwin

package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const platformSupported = true

// dataDirOK: the data folder (~/.local/share/firefox-vpn-client on Linux,
// ~/Library/Application Support/Firefox VPN Client on macOS) is on the
// app's drive, so the update's renames work, and outside the app (a
// portable copy keeps it inside, where the swap would move it along); the
// user may change the app's folder (not a copy that root installed, nor a
// macOS app opened from its disk image or from where macOS moved it to open
// it); and the core does not run as root, which must not install from a
// folder that the user can change meanwhile.
func dataDirOK(appDir, dataDir string) string {
	if os.Geteuid() == 0 {
		return "a copy run as root does not update itself"
	}
	var a, d syscall.Stat_t
	if syscall.Stat(appDir, &a) != nil || syscall.Stat(dataDir, &d) != nil {
		return "the app's or the data folder is missing"
	}
	if a.Dev != d.Dev {
		return "the data folder is not on the app's drive"
	}
	if rel, err := filepath.Rel(appDir, dataDir); err == nil && filepath.IsLocal(rel) {
		return "the data folder is inside the app"
	}
	if syscall.Access(appDir, 2 /* W_OK */) != nil {
		return "the app's folder is not writable"
	}
	return ""
}

// lockPath is the lock file that an update holds while it runs (Apply), in
// the data folder it installs from, which is the user's own: a core that
// uses that folder and starts meanwhile leaves the update's files alone.
func lockPath(dataDir string) string { return filepath.Join(dataDir, "update.lock") }

// updating reports whether an update that uses the data folder runs now.
func updating(dataDir string) bool {
	unlock, err := lockApp(dataDir)
	if err != nil {
		return errors.Is(err, errUpdateRunning)
	}
	unlock()
	return false
}

// lockApp takes the lock of updating(dataDir) until the returned function
// is called; it fails with errUpdateRunning while another update runs.
func lockApp(dataDir string) (func(), error) {
	f, err := os.OpenFile(lockPath(dataDir), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errUpdateRunning
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}

// waitForExit waits until the processes have exited, for at most timeout
// in all. A process that is gone already counts as exited; startTime tells
// a process from a later one that took its id.
func waitForExit(pids []int, timeout time.Duration) error {
	started := map[int]string{}
	for _, pid := range pids {
		if t := startTime(pid); t != "" {
			started[pid] = t
		}
	}
	deadline := time.Now().Add(timeout)
	for _, pid := range pids {
		t, ok := started[pid]
		if !ok {
			continue
		}
		for startTime(pid) == t {
			if time.Now().After(deadline) {
				return fmt.Errorf("the app did not close (process %d still runs)", pid)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}
