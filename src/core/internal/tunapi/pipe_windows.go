package tunapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// PipeName is the service's named pipe.
const PipeName = `\\.\pipe\firefox-vpn-tun`

// pipeName is PipeName; tests use their own.
var pipeName = PipeName

// ServiceName is the Windows service; ExeName its executable, which the app
// also carries in its core folder.
const (
	ServiceName = "FirefoxVPNTun"
	ExeName     = "firefox-vpn-tun.exe"
)

// ErrNoService means no service answers: it is not installed, or it is
// not running.
var ErrNoService = errors.New("the TUN service is not running")

// ErrNotAllowed means the service runs but this user did not install it:
// installing it again, as this user, lets them in.
var ErrNotAllowed = errors.New("the TUN service was installed for another user")

// clientAccess is what a client asks of the pipe: read, and write data.
// Not GENERIC_WRITE, whose FILE_APPEND_DATA is, on a pipe, the right to
// create another server end of it; the pipe's DACL gives users only this.
const clientAccess = windows.GENERIC_READ | windows.FILE_WRITE_DATA

// userAccess is clientAccess as the pipe's DACL grants it to users:
// FILE_GENERIC_READ and FILE_WRITE_DATA.
const userAccess = windows.READ_CONTROL | windows.SYNCHRONIZE | windows.FILE_READ_DATA |
	windows.FILE_READ_ATTRIBUTES | windows.FILE_READ_EA | windows.FILE_WRITE_DATA

// Dial connects to the service. It refuses a pipe that is not owned by
// the system or the administrators, which another program could have
// created under the name while the service was not running.
func Dial(ctx context.Context) (net.Conn, error) {
	c, err := winio.DialPipeAccess(ctx, pipeName, clientAccess)
	if err != nil {
		switch {
		case errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, os.ErrNotExist):
			return nil, ErrNoService
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			return nil, ErrNotAllowed
		}
		return nil, err
	}
	if err := checkOwner(c); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func checkOwner(c net.Conn) error {
	h, err := handleOf(c)
	if err != nil {
		return err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("reading the TUN service pipe's owner: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("reading the TUN service pipe's owner: %w", err)
	}
	if !owner.IsWellKnown(windows.WinLocalSystemSid) && !owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return fmt.Errorf("the TUN service pipe belongs to %s, not to the system", owner)
	}
	return nil
}

func handleOf(c net.Conn) (windows.Handle, error) {
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("not a pipe connection")
	}
	return windows.Handle(f.Fd()), nil
}

// Listen creates the service's pipe, which the system, the administrators,
// and the users with the given SIDs can open. The pipe is owned by the
// system, and creating it fails when another program holds the name.
func Listen(users []string) (net.Listener, error) {
	sddl, err := securityDescriptor("SY", nil, users)
	if err != nil {
		return nil, err
	}
	return listenSDDL(sddl)
}

// securityDescriptor is the pipe's, in SDDL: owned by owner; full access
// for the system, the administrators, and servers, which must be allowed
// to create the pipe's server ends; userAccess for users. Only the system
// may make itself the owner, so tests serve the pipe as their own user.
func securityDescriptor(owner string, servers, users []string) (string, error) {
	var sddl strings.Builder
	fmt.Fprintf(&sddl, "O:%sG:%sD:P(A;;GA;;;SY)(A;;GA;;;BA)", owner, owner)
	for _, sid := range servers {
		fmt.Fprintf(&sddl, "(A;;GA;;;%s)", sid)
	}
	for _, sid := range users {
		if _, err := windows.StringToSid(sid); err != nil {
			return "", fmt.Errorf("bad user SID %q: %w", sid, err)
		}
		fmt.Fprintf(&sddl, "(A;;0x%x;;;%s)", userAccess, sid)
	}
	return sddl.String(), nil
}

func listenSDDL(sddl string) (net.Listener, error) {
	return winio.ListenPipe(pipeName, &winio.PipeConfig{
		SecurityDescriptor: sddl,
		InputBufferSize:    64 << 10,
		OutputBufferSize:   64 << 10,
	})
}

// Installed reports whether the service is installed; it asks the service
// manager only what any user may ask.
func Installed() (bool, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, err
	}
	defer windows.CloseServiceHandle(m)
	name, _ := windows.UTF16PtrFromString(ServiceName)
	s, err := windows.OpenService(m, name, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	windows.CloseServiceHandle(s)
	return true, nil
}

// ClientProcess is the process at the other end of a connection the
// service accepted.
func ClientProcess(c net.Conn) (pid uint32, err error) {
	h, err := handleOf(c)
	if err != nil {
		return 0, err
	}
	err = windows.GetNamedPipeClientProcessId(h, &pid)
	return pid, err
}

// ProcessPath is the executable of process pid.
func ProcessPath(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// Allowed reports whether the client of a connection the service accepted
// may use it: the pipe's access list let in only those who may.
func Allowed(net.Conn) bool { return true }
