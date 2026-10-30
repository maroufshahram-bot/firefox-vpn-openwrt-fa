package tunservice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// InstallForUser installs the service for the user running this process.
// Windows' prompt names the program itself, so prompt (macOS') is not
// used.
func InstallForUser(prompt string) error {
	user, err := currentUser()
	if err != nil {
		return err
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		return Install(user)
	}
	return runElevated("install-as-admin", "-user", user)
}

// UninstallForUser removes the service: through its pipe while it runs,
// with no prompt; else elevated.
func UninstallForUser(prompt string) error {
	if installed, err := tunapi.Installed(); err == nil && !installed {
		return nil
	}
	if err := askService(tunapi.Message{Type: tunapi.TypeUninstall}, tunapi.TypeUninstalling); err == nil {
		return waitRemoved(30 * time.Second)
	}
	if windows.GetCurrentProcessToken().IsElevated() {
		return Uninstall()
	}
	return runElevated("uninstall-as-admin")
}

func currentUser() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// runElevated runs this executable with args as an administrator, after
// Windows' prompt, and waits for it. The elevated copy writes why it
// failed to a report file, since it has no console of this process.
func runElevated(args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	report, err := os.CreateTemp("", "firefox-vpn-tun-*.txt")
	if err != nil {
		return err
	}
	report.Close()
	defer os.Remove(report.Name())
	args = append(args, "-report", report.Name())
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = windows.EscapeArg(a)
	}
	code, err := shellRunAs(self, strings.Join(quoted, " "))
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		return err
	}
	if code == 0 {
		return nil
	}
	if text := readReport(report.Name()); text != "" {
		return errors.New(text)
	}
	return fmt.Errorf("%s failed with exit code %d", filepath.Base(self), code)
}

// shellRunAs starts file with params through ShellExecuteEx's "runas",
// which asks for administrator rights, and returns its exit code. The
// prompt belongs to the foreground window: the app or Setup that asked.
func shellRunAs(file, params string) (uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil {
		defer windows.CoUninitialize()
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	p, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{
		mask:       seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		hwnd:       windows.GetForegroundWindow(),
		verb:       verb,
		file:       f,
		parameters: p,
		show:       windows.SW_HIDE,
	}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0, e
	}
	if info.process == 0 {
		return 0, errors.New("the elevated process did not start")
	}
	defer windows.CloseHandle(info.process)
	if _, err := windows.WaitForSingleObject(info.process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return code, nil
}

var procShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

const (
	seeMaskNoCloseProcess = 0x40
	seeMaskNoAsync        = 0x100
	seeMaskFlagNoUI       = 0x400
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size          uint32
	mask          uint32
	hwnd          windows.HWND
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}
