package update

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// On Linux the app is the release's tar.gz.
func init() { thePackage = linuxPackage }

// startTime is a process's start time from /proc/<pid>/stat (field 22), so
// a finished process's id that another program took meanwhile does not
// count as the same process; "" when there is no such process, or it has
// ended and waits for its parent (a zombie).
func startTime(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	// The command name in parentheses may hold spaces: count after it.
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return ""
	}
	fields := bytes.Fields(data[end+1:])
	if len(fields) < 20 || string(fields[0]) == "Z" || string(fields[0]) == "X" {
		return ""
	}
	return string(fields[19])
}

// scopeStartWait is how long systemd-run may take to fail: it then started
// nothing, and the program is started without it.
var scopeStartWait = time.Second

// detached starts name with args apart from this process: in a systemd
// scope of its own when the user's systemd has one to give, so it outlives
// the unit the app ran in (KDE starts each app in one, whose end takes its
// processes along), else in a session of its own.
func detached(dir, name string, args ...string) error {
	if run, err := exec.LookPath("systemd-run"); err == nil && os.Getenv("XDG_RUNTIME_DIR") != "" {
		cmd := exec.Command(run, append([]string{"--user", "--scope", "--quiet", "--collect", "--", name}, args...)...)
		cmd.Dir = dir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err == nil {
			// systemd-run becomes the program once its scope is made; it
			// exits at once when it cannot make one or start the program.
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			select {
			case err := <-exited:
				if err == nil {
					return nil
				}
			case <-time.After(scopeStartWait):
				return nil
			}
		}
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// startUpdater starts the copy of the core at exe with -update, apart from
// this process and the app, its working folder outside the app's.
func startUpdater(exe, dataDir string) error {
	return detached(filepath.Dir(exe), exe, "-update", "-data-dir", dataDir)
}

// startApp starts the app in appDir with args.
func startApp(appDir string, args []string) error {
	return detached(appDir, filepath.Join(appDir, thePackage.app), args...)
}

// setInstalledVersion: Linux keeps no list of installed apps to update.
func setInstalledVersion(string, string) error { return nil }
