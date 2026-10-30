package update

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeProcesses replaces the processes Apply waits for and starts, and
// Windows' list of installed apps.
type fakeProcesses struct {
	waited  []int
	waitErr error
	started []string
	marked  []string
}

func (f *fakeProcesses) install(t *testing.T) {
	t.Helper()
	waitExited = func(pids []int, _ time.Duration) error {
		f.waited = pids
		return f.waitErr
	}
	launchApp = func(appDir string, args []string) error {
		f.started = append([]string{appDir}, args...)
		return nil
	}
	markInstalled = func(appDir, v string) error {
		f.marked = []string{appDir, v}
		return nil
	}
	t.Cleanup(func() {
		waitExited, launchApp, markInstalled = waitForExit, startApp, setInstalledVersion
	})
}

func readResult(t *testing.T, dataDir string) Result {
	t.Helper()
	var r Result
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dataDir, "update", "result.json"))), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestApplySwapsAPortableCopyAndStartsTheApp(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	installed(t, appDir, dataDir, "--autostart")
	var f fakeProcesses
	f.install(t)
	if err := Apply(dataDir); err != nil {
		t.Fatal(err)
	}
	if len(f.waited) != 2 || f.waited[0] != 4242 {
		t.Fatalf("waited for %v", f.waited)
	}
	if read(t, filepath.Join(appDir, "core", "firefox-vpn-core.exe")) != "core 1.2.3" || read(t, filepath.Join(appDir, "user-data", "config.json")) != "the user's settings" {
		t.Fatal("not swapped")
	}
	if r := readResult(t, dataDir); !r.OK || r.From != "1.0.0" || r.To != "1.2.3" || !r.On {
		t.Fatalf("%+v", r)
	}
	if strings.Join(f.started, " ") != appDir+" --autostart" {
		t.Fatalf("started %v", f.started)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "update", "pending.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the instructions stayed")
	}
	// The new version's core reads the result and cleans up.
	m := New(Options{DataDir: dataDir, AppDir: appDir, Current: "1.2.3"})
	if s := m.Status(); s.Last == nil || !s.Last.OK {
		t.Fatalf("%+v", s)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "update", "backup")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the old files stayed")
	}
}

// A copy Setup installed is swapped the same way, keeps Setup's
// uninstaller, and gets its new version in Windows' list.
func TestApplySwapsAnInstalledCopyAndSetsItsVersion(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", true)
	installed(t, appDir, dataDir)
	var f fakeProcesses
	f.install(t)
	if err := Apply(dataDir); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(appDir, "core", "firefox-vpn-core.exe")) != "core 1.2.3" || read(t, filepath.Join(appDir, "unins000.exe")) != "uninstaller" {
		t.Fatal("not swapped, or the uninstaller went")
	}
	if strings.Join(f.marked, " ") != appDir+" 1.2.3" {
		t.Fatalf("marked %v", f.marked)
	}
}

// After a failed swap the old version starts and says why, and Windows'
// list keeps its version.
func TestAFailedApplyStartsTheOldVersion(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", true)
	installed(t, appDir, dataDir)
	renameWait = 50 * time.Millisecond
	rename = func(from, to string) error {
		if filepath.Base(from) == "data" && strings.Contains(from, filepath.Join("update", "new")) {
			return errors.New("in use")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { rename, renameWait = os.Rename, 10*time.Second })
	var f fakeProcesses
	f.install(t)
	if err := Apply(dataDir); err == nil {
		t.Fatal("no error")
	}
	if read(t, filepath.Join(appDir, "core", "firefox-vpn-core.exe")) != "core 1.0.0" || read(t, filepath.Join(appDir, "data", "app.so")) != "assets 1.0.0" {
		t.Fatal("the old version is not back")
	}
	if r := readResult(t, dataDir); r.OK || !strings.Contains(r.Error, "data") {
		t.Fatalf("%+v", r)
	}
	if len(f.started) != 1 || f.started[0] != appDir || f.marked != nil {
		t.Fatalf("started %v, marked %v", f.started, f.marked)
	}
}

func TestApplyLeavesARunningAppAlone(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	installed(t, appDir, dataDir)
	var f fakeProcesses
	f.waitErr = errors.New("the app did not close")
	f.install(t)
	if err := Apply(dataDir); err == nil {
		t.Fatal("no error")
	}
	if read(t, filepath.Join(appDir, "firefox-vpn.exe")) != "app 1.0.0" || f.started != nil {
		t.Fatal("changed a running app")
	}
	if r := readResult(t, dataDir); r.OK {
		t.Fatalf("%+v", r)
	}
}

func TestWaitForExitWaitsForRealProcesses(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	// This process does not exit: the wait times out. A process id that
	// does not exist counts as exited.
	if err := waitForExit([]int{os.Getpid()}, 50*time.Millisecond); err == nil {
		t.Fatal("this process counted as exited")
	}
	if err := waitForExit([]int{1 << 30}, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyOneUpdateOfAnAppRunsAtATime(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	dataDir := t.TempDir()
	unlock, err := lockApp(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !updating(dataDir) || updating(t.TempDir()) {
		t.Fatal("updating() is wrong")
	}
	if _, err := lockApp(dataDir); !errors.Is(err, errUpdateRunning) {
		t.Fatalf("locked twice: %v", err)
	}
	unlock()
	if updating(dataDir) {
		t.Fatal("still updating")
	}
}

// Beside another update of the same data folder, Apply does nothing: that
// update starts the app again.
func TestApplyLeavesTheAppToAnotherUpdate(t *testing.T) {
	if !platformSupported {
		t.Skip("Windows, Linux, and macOS only")
	}
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	installed(t, appDir, dataDir)
	var f fakeProcesses
	f.install(t)
	unlock, err := lockApp(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := Apply(dataDir); !errors.Is(err, errUpdateRunning) || f.started != nil || f.waited != nil {
		t.Fatalf("Apply: %v, waited %v, started %v", err, f.waited, f.started)
	}
}
