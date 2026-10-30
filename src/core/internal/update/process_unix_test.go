//go:build linux || darwin

package update

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// A running process has a start time that stays; one that has ended has
// none, also before its parent took its exit status (a zombie), so the
// update does not wait for it.
func TestAnEndedProcessHasNoStartTime(t *testing.T) {
	if a, b := startTime(os.Getpid()), startTime(os.Getpid()); a == "" || a != b {
		t.Fatalf("this process: %q %q", a, b)
	}
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	for deadline := time.Now().Add(5 * time.Second); startTime(cmd.Process.Pid) != ""; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("an ended process still has a start time")
		}
	}
	if err := waitForExit([]int{cmd.Process.Pid}, time.Second); err != nil {
		t.Fatal(err)
	}
}

// A lock that cannot be taken (here a folder in its place) is no update
// running, and does not stop the update: the app comes back.
func TestApplyGoesOnWhenTheLockCannotBeTaken(t *testing.T) {
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	installed(t, appDir, dataDir)
	var f fakeProcesses
	f.install(t)
	if err := os.Remove(lockPath(dataDir)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lockPath(dataDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if updating(dataDir) {
		t.Fatal("a lock that cannot be taken counts as an update running")
	}
	if err := Apply(dataDir); err != nil {
		t.Fatal(err)
	}
	if len(f.started) == 0 || !readResult(t, dataDir).OK {
		t.Fatalf("started %v", f.started)
	}
}
