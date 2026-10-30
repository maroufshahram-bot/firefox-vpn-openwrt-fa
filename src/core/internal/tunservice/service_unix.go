//go:build linux || darwin

package tunservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunstack"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// The installed service lives in a folder only root may write to
// (InstallDir): it runs as root, so nobody else may replace its file. The
// app carries the same executable beside its core, and installs it from
// there. The init system (systemd, launchd; service_linux.go,
// service_darwin.go) starts it at boot and again whenever it ends but by
// an uninstall, which is how an update's new file starts.
const (
	newExeName = tunapi.ExeName + ".new"
	// usersFile lists the user ids that may use the service, one a line.
	usersFile = "users"
)

// stateDir is the service's folder, where the filters note what they
// changed (filters_darwin.go).
var stateDir = InstallDir

// RunService runs this executable as the service until the init system
// stops it, or an update or uninstall ends it.
func RunService() error {
	if os.Geteuid() != 0 {
		return errors.New("the service runs as root")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(self)
	stateDir = dir
	logx.EnableFile(filepath.Join(dir, "logs"), false)
	_ = os.Remove(filepath.Join(dir, newExeName))
	// What a TUN that ended with a crash left: its rules on Linux, the DNS
	// settings on macOS.
	if err := tunstack.CleanupStale(); err != nil {
		logx.Warn("removing the rules an earlier TUN left failed err=%s", logx.Err(err))
	}
	if err := restoreFilters(); err != nil {
		logx.Warn("setting back what an earlier TUN changed failed err=%s", logx.Err(err))
	}
	server := NewServer(Config{
		Version:   version.Current,
		Self:      self,
		Update:    func(path string) error { return applyUpdate(self, path) },
		Uninstall: func() error { return removeSelf(dir) },
	})
	served := make(chan struct{})
	go func() {
		defer close(served)
		serve(server, func() []string { return readUsers(dir) })
	}()
	logx.Info("TUN service started version=%s pid=%d", version.Current, os.Getpid())
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	reason := ""
	select {
	case sig := <-signals:
		reason = "signal " + sig.String()
	case <-server.Ended():
		reason = server.Reason()
	}
	logx.Info("TUN service stopping reason=%s", reason)
	server.Close()
	select {
	case <-served:
	case <-time.After(10 * time.Second):
		logx.Warn("the TUN service did not stop in time")
	}
	if reason == "uninstalled" {
		// The init system stops it for good; until then it waits.
		stopForGood()
	}
	return nil
}

// readUsers lists the user ids that may use the service.
func readUsers(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, usersFile))
	if err != nil {
		return nil
	}
	var users []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			users = append(users, line)
		}
	}
	return users
}

// addUser lets user id uid use the service.
func addUser(dir, uid string) error {
	if _, err := strconv.ParseUint(uid, 10, 32); err != nil {
		return fmt.Errorf("bad user id %q", uid)
	}
	users := readUsers(dir)
	for _, u := range users {
		if u == uid {
			return nil
		}
	}
	return replaceFile(filepath.Join(dir, usersFile), []byte(strings.Join(append(users, uid), "\n")+"\n"), 0o644)
}

// Install installs this executable as the service, or replaces the one
// installed, lets user id uid use it, and starts it. It needs root.
func Install(uid string) error {
	if os.Geteuid() != 0 {
		return errors.New("installing the service needs root")
	}
	if err := initSystemPresent(); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir := InstallDir
	target := filepath.Join(dir, tunapi.ExeName)
	if err := stopInstalled(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "logs"), 0o755); err != nil {
		return err
	}
	if filepath.Clean(self) != filepath.Clean(target) {
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := replaceFile(target, data, 0o755); err != nil {
			return fmt.Errorf("copying the service to %s: %w", dir, err)
		}
	}
	if err := addUser(dir, uid); err != nil {
		return err
	}
	if err := register(target); err != nil {
		return err
	}
	return waitForService(15 * time.Second)
}

// Uninstall stops and removes the service and its folder. It needs root;
// a running service removes itself (removeSelf) when asked through its
// socket.
func Uninstall() error {
	if os.Geteuid() != 0 {
		return errors.New("removing the service needs root")
	}
	stopErr := stopInstalled()
	// The service sets them back as it stops; after a crash it could not.
	if err := restoreFilters(); err != nil {
		stopErr = errors.Join(stopErr, err)
	}
	if err := unregister(); err != nil {
		return err
	}
	if err := removeFolder(InstallDir); err != nil {
		return err
	}
	return stopErr
}

// removeSelf removes the service from inside it: its unit or job file and
// its folder (a running executable may be deleted); RunService then has the
// init system stop it for good.
func removeSelf(dir string) error {
	if err := unregister(); err != nil {
		return err
	}
	return removeFolder(dir)
}

// removeFolder removes the service's folder, never another.
func removeFolder(dir string) error {
	if filepath.Clean(dir) != filepath.Clean(InstallDir) {
		return fmt.Errorf("refusing to remove %s", dir)
	}
	return os.RemoveAll(dir)
}

// applyUpdate makes the signed executable at path the service's: written
// beside the running one from the bytes that were checked, not older, it
// takes the running file's name (the running process keeps the old file
// open). The init system starts it once this process has ended.
func applyUpdate(self, path string) error {
	data, err := readSigned(path)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(self)
	if err == nil && bytes.Equal(current, data) {
		return errors.New("the service is already this build")
	}
	next := filepath.Join(filepath.Dir(self), newExeName)
	if err := replaceFile(next, data, 0o755); err != nil {
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
	if err := os.Rename(next, self); err != nil {
		os.Remove(next)
		return err
	}
	return nil
}

// run runs a system tool and returns its error with what it said.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if text := strings.TrimSpace(string(out)); text != "" {
			return fmt.Errorf("%s %s: %s", filepath.Base(name), strings.Join(args, " "), text)
		}
		return fmt.Errorf("%s %s: %w", filepath.Base(name), strings.Join(args, " "), err)
	}
	return nil
}
