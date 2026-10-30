package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
)

// exitWait bounds the wait for the app and the core to exit.
const exitWait = 2 * time.Minute

// The processes an update starts and waits for, and what the system keeps
// about the app (process_*.go); tests replace them.
var (
	launchUpdater = startUpdater
	waitExited    = waitForExit
	launchApp     = startApp
	markInstalled = setInstalledVersion
)

// Apply is the -update run of the copy of the core that Install started:
// it waits until the app and its core have exited, swaps the app's files
// for the new version's, writes the result for the next start, and starts
// the app again. On any failure the old version is put back and starts,
// and the result says why.
func Apply(dataDir string) error {
	dir := filepath.Join(dataDir, "update")
	data, err := os.ReadFile(filepath.Join(dir, "pending.json"))
	if err != nil {
		return fmt.Errorf("nothing to install: %w", err)
	}
	var p pending
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("unreadable instructions: %w", err)
	}
	if p.AppDir == "" || p.Package == "" {
		return errors.New("incomplete instructions")
	}
	logx.Info("update: installing from=%s to=%s app=%s", p.From, p.To, p.AppDir)
	unlock, err := lockApp(dataDir)
	if errors.Is(err, errUpdateRunning) {
		// That update starts the app again.
		return err
	}
	if err != nil {
		// The lock only keeps a core started meanwhile from cleaning up;
		// without it the update still runs and the app comes back.
		logx.Warn("update: going on without the lock err=%s", logx.Err(err))
		unlock = func() {}
	}
	result := Result{From: p.From, To: p.To, On: p.On}
	waitErr := waitExited(p.Wait, exitWait)
	err = waitErr
	if err == nil {
		err = swap(p.AppDir, p.Package, filepath.Join(dir, "backup"))
	}
	result.OK = err == nil
	if err != nil {
		result.Error = err.Error()
		logx.Error("update: failed err=%s", logx.Err(err))
	} else {
		logx.Info("update: installed version=%s", p.To)
		// What the system keeps about the app: Windows' list of installed
		// apps shows the new version of a copy Setup installed; on macOS
		// the bundle loses the mark of a downloaded app.
		if merr := markInstalled(p.AppDir, p.To); merr != nil {
			logx.Warn("update: telling the system about the new version failed err=%s", logx.Err(merr))
		}
	}
	if out, merr := json.MarshalIndent(result, "", "  "); merr == nil {
		if werr := os.WriteFile(filepath.Join(dir, "result.json"), out, 0o644); werr != nil {
			logx.Error("update: writing the result failed err=%s", logx.Err(werr))
		}
	}
	os.Remove(filepath.Join(dir, "pending.json"))
	// The next core cleans up only once this run is over.
	unlock()
	if waitErr != nil {
		// The app still runs; it is not started twice.
		return err
	}
	if serr := launchApp(p.AppDir, p.Args); serr != nil {
		logx.Error("update: starting the app failed err=%s", logx.Err(serr))
		return errors.Join(err, serr)
	}
	return err
}

// renameWait is how long a rename is tried again while a file is in use
// (a virus scanner reading a new file, say); tests shorten it.
var renameWait = 10 * time.Second

// rename is os.Rename; tests replace it to fail.
var rename = os.Rename

// renameRetry renames from to to, trying again for renameWait.
func renameRetry(from, to string) error {
	deadline := time.Now().Add(renameWait)
	for {
		err := rename(from, to)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// kept are the names in the app's folder that an update never replaces:
// the data, the portable marker, and Setup's uninstaller.
var kept = map[string]bool{"user-data": true, "portable": true, "unins000.exe": true, "unins000.dat": true}

// swap puts each file and folder of newDir in appDir in place of the one of
// the same name, which moves to backupDir; anything else in appDir stays.
// All are renames on one drive, so a swap takes a moment and the old files
// stay whole. On a failure everything is moved back, and the error says
// what failed.
func swap(appDir, newDir, backupDir string) (err error) {
	entries, err := os.ReadDir(newDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("the new version is empty")
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	type move struct{ from, to string }
	var done []move
	defer func() {
		if err == nil {
			return
		}
		logx.Warn("update: putting the old version back err=%s", logx.Err(err))
		for i := len(done) - 1; i >= 0; i-- {
			if rerr := renameRetry(done[i].to, done[i].from); rerr != nil {
				err = fmt.Errorf("%w; putting %s back failed too: %v", err, filepath.Base(done[i].from), rerr)
			}
		}
	}()
	for _, e := range entries {
		name := e.Name()
		if kept[name] {
			continue
		}
		target := filepath.Join(appDir, name)
		if _, statErr := os.Lstat(target); statErr == nil {
			aside := filepath.Join(backupDir, name)
			if err = renameRetry(target, aside); err != nil {
				return fmt.Errorf("moving the old %s aside: %w", name, err)
			}
			done = append(done, move{target, aside})
		}
		from := filepath.Join(newDir, name)
		if err = renameRetry(from, target); err != nil {
			return fmt.Errorf("putting the new %s in place: %w", name, err)
		}
		done = append(done, move{from, target})
	}
	return nil
}
