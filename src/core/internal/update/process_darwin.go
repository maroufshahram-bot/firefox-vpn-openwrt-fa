package update

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
)

// On macOS the app is its bundle (Firefox VPN.app), in the release's
// tar.gz.
func init() { thePackage = macPackage }

// startTime is a process's start time (sysctl kern.proc.pid), so a finished
// process's id that another program took meanwhile does not count as the
// same process; "" when there is no such process, or it has ended and
// waits for its parent (a zombie).
func startTime(pid int) string {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(p.Proc.P_pid) != pid || p.Proc.P_stat == 5 /* SZOMB */ {
		return ""
	}
	t := p.Proc.P_starttime
	return fmt.Sprintf("%d.%06d", t.Sec, t.Usec)
}

// startUpdater starts the copy of the core at exe with -update, apart from
// this process and the app: in a session of its own, as launchd ends the
// process group of an app that its LaunchAgent started at sign-in with it.
func startUpdater(exe, dataDir string) error {
	cmd := exec.Command(exe, "-update", "-data-dir", dataDir)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// openWait bounds open(1), which returns once Launch Services has started
// the app.
var openWait = 30 * time.Second

// startApp opens the app (its bundle, appDir) with args as the Finder
// would, through Launch Services: a new instance, behind the other windows
// when it starts in the menu bar. When open fails, it starts the app's
// executable itself.
func startApp(appDir string, args []string) error {
	openArgs := []string{"-n"}
	if slices.Contains(args, "--hidden") || slices.Contains(args, "--autostart") {
		openArgs = append(openArgs, "-g")
	}
	openArgs = append(openArgs, appDir)
	if len(args) > 0 {
		openArgs = append(append(openArgs, "--args"), args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), openWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/open", openArgs...).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		// The app may still start: not a second time.
		return fmt.Errorf("opening the app did not finish: %w", err)
	}
	logx.Warn("update: opening the app failed, starting it directly err=%s output=%q", logx.Err(err), strings.TrimSpace(string(out)))
	cmd := exec.Command(filepath.Join(appDir, filepath.FromSlash(thePackage.app)), args...)
	cmd.Dir = appDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// quarantine is the mark macOS keeps on a downloaded file; the Finder's
// copy from the disk image leaves it on the app's bundle.
const quarantine = "com.apple.quarantine"

// setInstalledVersion: macOS keeps no list of installed apps. The bundle
// loses the mark of a downloaded app, which Gatekeeper cleared when the user
// first opened it, so the new version (which the release's signature
// vouched for) opens without macOS asking again.
func setInstalledVersion(appDir, _ string) error {
	if err := unix.Removexattr(appDir, quarantine); err != nil && !errors.Is(err, unix.ENOATTR) {
		return err
	}
	return nil
}
