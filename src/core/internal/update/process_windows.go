package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const platformSupported = true

// dataDirOK: the data folder is the app's own user-data, so the update's
// renames stay on one drive.
func dataDirOK(appDir, dataDir string) string {
	if !strings.EqualFold(filepath.Clean(dataDir), filepath.Join(filepath.Clean(appDir), "user-data")) {
		return "the data folder is not the app's own (user-data beside it)"
	}
	return ""
}

// mutexName names the lock that an update holds while it runs (Apply), by
// the data folder it installs from (the app's user-data), so a core that
// uses that folder and starts meanwhile leaves the update's files alone.
func mutexName(dataDir string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return `Local\FirefoxVPNUpdate-` + hex.EncodeToString(sum[:8])
}

// updating reports whether an update that uses the data folder runs now.
func updating(dataDir string) bool {
	name, err := windows.UTF16PtrFromString(mutexName(dataDir))
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

// lockApp takes the lock of updating(dataDir) until the returned function
// is called; it fails with errUpdateRunning while another update runs.
func lockApp(dataDir string) (func(), error) {
	name, err := windows.UTF16PtrFromString(mutexName(dataDir))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return nil, errUpdateRunning
		}
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}

// waitForExit waits until the processes have exited, for at most timeout
// in all. A process that is gone already counts as exited. The processes
// are opened before any wait, so a finished one's id cannot be taken by
// another program in between.
func waitForExit(pids []int, timeout time.Duration) error {
	type proc struct {
		pid int
		h   windows.Handle
	}
	var procs []proc
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			continue
		}
		procs = append(procs, proc{pid, h})
	}
	defer func() {
		for _, p := range procs {
			windows.CloseHandle(p.h)
		}
	}()
	deadline := time.Now().Add(timeout)
	for _, p := range procs {
		left := max(time.Until(deadline), 0)
		ev, err := windows.WaitForSingleObject(p.h, uint32(left/time.Millisecond))
		if err != nil {
			return fmt.Errorf("waiting for process %d: %w", p.pid, err)
		}
		if ev != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("the app did not close (process %d still runs)", p.pid)
		}
	}
	return nil
}

// uninstallKey is Setup's entry in Windows' list of installed apps, for the
// current user: its AppId (app/windows/installer/firefox-vpn.iss) and "_is1".
// Tests use a key of their own.
var uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\{6F3A2C1E-9B4D-4E7A-8C21-5D0F7B9E4A63}_is1`

// setInstalledVersion sets version v in Setup's entry when Setup installed
// the app in appDir; nothing for a portable copy (no entry, or one for
// another folder).
func setInstalledVersion(appDir, v string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	location, _, err := k.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(filepath.Clean(location), filepath.Clean(appDir)) {
		return nil
	}
	if err := k.SetStringValue("DisplayVersion", v); err != nil {
		return err
	}
	parts := strings.Split(v, ".")
	part := func(i int) uint32 {
		if i >= len(parts) {
			return 0
		}
		n, _ := strconv.ParseUint(parts[i], 10, 32)
		return uint32(n)
	}
	for name, value := range map[string]uint32{"MajorVersion": part(0), "VersionMajor": part(0), "MinorVersion": part(1), "VersionMinor": part(1)} {
		if err := k.SetDWordValue(name, value); err != nil {
			return err
		}
	}
	return nil
}

// startUpdater starts the copy of the core at exe with -update, apart from
// this process: no console, its own process group, its working folder
// outside the app's (an open working folder could not be renamed).
func startUpdater(exe, dataDir string) error {
	cmd := exec.Command(exe, "-update", "-data-dir", dataDir)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// startApp starts the app in appDir with args.
func startApp(appDir string, args []string) error {
	cmd := exec.Command(filepath.Join(appDir, thePackage.app), args...)
	cmd.Dir = appDir
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
