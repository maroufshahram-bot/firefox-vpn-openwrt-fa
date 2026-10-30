package tunservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// The installed service lives in Program Files, where only administrators
// may write: it runs with the system's rights, so nobody else may replace
// its file or put a library beside it. The app carries the same
// executable in its core folder, and installs it from there.
const (
	installFolder = "Firefox VPN"
	displayName   = "Firefox VPN TUN"
	description   = "Runs the TUN mode of Firefox VPN."
	newExeName    = "firefox-vpn-tun.new.exe"
	oldExeName    = "firefox-vpn-tun.old.exe"
)

// usersKey holds the SIDs of the users who installed the service, the
// only ones (with the system and administrators) who may open its pipe.
const usersKey = `SYSTEM\CurrentControlSet\Services\` + tunapi.ServiceName + `\Parameters`

// InstallDir is the installed service's folder.
func InstallDir() (string, error) {
	pf, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(pf, installFolder), nil
}

// RunService runs this executable as the service until the service
// manager stops it, or an update or uninstall ends it.
func RunService() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(self)
	logx.EnableFile(filepath.Join(dir, "logs"), false)
	// Left by the update that started this executable.
	_ = os.Remove(filepath.Join(dir, oldExeName))
	return svc.Run(tunapi.ServiceName, &serviceHandler{self: self})
}

type serviceHandler struct{ self string }

func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	server := NewServer(Config{
		Version:   version.Current,
		Self:      h.self,
		Update:    func(path string) error { return applyUpdate(h.self, path) },
		Uninstall: func() error { return removeSelf(h.self) },
	})
	served := make(chan struct{})
	go func() {
		defer close(served)
		serve(server, allowedUsers)
	}()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	logx.Info("TUN service started version=%s pid=%d", version.Current, os.Getpid())
	stop := func(reason string) {
		status <- svc.Status{State: svc.StopPending}
		logx.Info("TUN service stopping reason=%s", reason)
		server.Close()
		select {
		case <-served:
		case <-time.After(10 * time.Second):
			logx.Warn("the TUN service did not stop in time")
		}
	}
	for {
		select {
		case c := <-requests:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop:
				stop("stop requested")
				return false, 0
			case svc.Shutdown:
				stop("Windows is shutting down")
				return false, 0
			}
		case <-server.Ended():
			stop(server.Reason())
			return false, 0
		}
	}
}

func allowedUsers() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, usersKey, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	users, _, _ := k.GetStringsValue("Users")
	return users
}

func addUser(sid string) error {
	if _, err := windows.StringToSid(sid); err != nil {
		return fmt.Errorf("bad user SID %q: %w", sid, err)
	}
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, usersKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	users, _, _ := k.GetStringsValue("Users")
	for _, u := range users {
		if strings.EqualFold(u, sid) {
			return nil
		}
	}
	return k.SetStringsValue("Users", append(users, sid))
}

// Install installs this executable as the service, or replaces the one
// installed, lets the user with SID user use it, and starts it. It needs
// administrator rights.
func Install(user string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := InstallDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, tunapi.ExeName)
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(tunapi.ServiceName)
	exists := err == nil
	if exists {
		defer s.Close()
		if err := stopService(s); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := replaceFile(target, data, 0o755); err != nil {
			return fmt.Errorf("copying the service to %s: %w", dir, err)
		}
	}
	cfg := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  displayName,
		Description:  description,
	}
	if exists {
		cfg.BinaryPathName = windows.EscapeArg(target) + " service"
		if err := s.UpdateConfig(cfg); err != nil {
			return fmt.Errorf("updating the service: %w", err)
		}
	} else {
		s, err = m.CreateService(tunapi.ServiceName, target, cfg, "service")
		if err != nil {
			return fmt.Errorf("creating the service: %w", err)
		}
		defer s.Close()
	}
	// A crash restarts it; an update or an uninstall ends it cleanly.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 2 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 24*60*60); err != nil {
		return fmt.Errorf("setting the service's recovery: %w", err)
	}
	if err := addUser(user); err != nil {
		return err
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("starting the service: %w", err)
	}
	return waitForService(15 * time.Second)
}

func stopService(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	if st.State != svc.StopPending {
		if _, err := s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return fmt.Errorf("stopping the service: %w", err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == svc.Stopped {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the service does not stop")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Uninstall stops and removes the service and its folder. It needs
// administrator rights; a running service removes itself (removeSelf)
// when asked through its pipe.
func Uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to the service manager: %w", err)
	}
	defer m.Disconnect()
	if s, err := m.OpenService(tunapi.ServiceName); err == nil {
		stopErr := stopService(s)
		delErr := s.Delete()
		s.Close()
		if delErr != nil && !errors.Is(delErr, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return fmt.Errorf("removing the service: %w", delErr)
		}
		if stopErr != nil {
			return stopErr
		}
	}
	dir, err := InstallDir()
	if err != nil {
		return err
	}
	return removeFolder(dir)
}

// removeSelf removes the service from inside it: the service manager
// deletes it once it stops, and a copy of this executable outside the
// folder removes the folder once this process has ended.
func removeSelf(self string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(tunapi.ServiceName)
	if err != nil {
		return err
	}
	err = s.Delete()
	s.Close()
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return err
	}
	return startDetached(self, "cleanup", true, strconv.Itoa(os.Getpid()), filepath.Dir(self))
}

// Cleanup waits for process pid to end, then removes the service's folder
// dir and schedules this executable, a temporary copy, for deletion.
func Cleanup(pid int, dir string) error {
	waitForProcess(pid, 30*time.Second)
	err := removeFolder(dir)
	if self, e := os.Executable(); e == nil {
		p, _ := windows.UTF16PtrFromString(self)
		_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	return err
}

// removeFolder removes the service's folder, never another: only the
// Firefox VPN folder in Program Files.
func removeFolder(dir string) error {
	want, err := InstallDir()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(dir), filepath.Clean(want)) {
		return fmt.Errorf("refusing to remove %s", dir)
	}
	var last error
	for range 20 {
		if last = os.RemoveAll(dir); last == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return last
}

// applyUpdate makes the signed executable at path the service's: it is
// copied into the service's folder from the bytes that were checked, must
// not be older, and takes the running file's name; a copy of it restarts
// the service once this process has ended.
func applyUpdate(self, path string) error {
	data, err := readSigned(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(self)
	if err == nil && bytes.Equal(current, data) {
		return errors.New("the service is already this build")
	}
	dir := filepath.Dir(self)
	next := filepath.Join(dir, newExeName)
	if err := os.WriteFile(next, data, 0o644); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	out, err := exec.CommandContext(ctx, next, "-version").Output()
	cancel()
	if err != nil {
		os.Remove(next)
		return fmt.Errorf("the update does not run: %w", err)
	}
	if v := strings.TrimSpace(string(out)); version.Newer(version.Current, v) {
		os.Remove(next)
		return fmt.Errorf("the update is version %s, older than %s", v, version.Current)
	}
	old := filepath.Join(dir, oldExeName)
	_ = os.Remove(old)
	// A running executable may be renamed, not replaced.
	if err := os.Rename(self, old); err != nil {
		os.Remove(next)
		return err
	}
	if err := os.Rename(next, self); err != nil {
		_ = os.Rename(old, self)
		os.Remove(next)
		return err
	}
	return startDetached(self, "restart-service", false, strconv.Itoa(os.Getpid()))
}

// RestartService waits for process pid (the service before an update) to
// end, then starts the service again.
func RestartService(pid int) error {
	waitForProcess(pid, 30*time.Second)
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(tunapi.ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := s.Start()
		if err == nil || errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func waitForProcess(pid int, limit time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // already gone
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(limit.Milliseconds()))
}

// startDetached starts exe (or, with copy, a copy of it in the system's
// temporary folder) with args, to outlive this process.
func startDetached(exe, command string, copyFirst bool, args ...string) error {
	if copyFirst {
		data, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp(os.TempDir(), "firefox-vpn-tun-cleanup-*.exe")
		if err != nil {
			return err
		}
		_, werr := tmp.Write(data)
		cerr := tmp.Close()
		if werr != nil || cerr != nil {
			os.Remove(tmp.Name())
			return errors.Join(werr, cerr)
		}
		exe = tmp.Name()
	}
	cmd := exec.Command(exe, append([]string{command}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
