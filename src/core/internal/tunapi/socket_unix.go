//go:build linux || darwin

package tunapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strconv"
	"syscall"
)

// ServiceName names the service (the systemd unit, or launchd's label's
// end); ExeName is its executable, which the app also carries beside its
// core.
const (
	ServiceName = "firefox-vpn-tun"
	ExeName     = "firefox-vpn-tun"
)

// ErrNoService means no service answers: it is not installed, or it is
// not running.
var ErrNoService = errors.New("the TUN service is not running")

// ErrNotAllowed means the service runs but this user did not install it:
// installing it again, as this user, lets them in.
var ErrNotAllowed = errors.New("the TUN service was installed for another user")

// socketPath is SocketPath; tests use their own.
var socketPath = SocketPath

// serverUID is the user the socket's server must be: root. Tests serve it
// as themselves.
var serverUID = 0

// Dial connects to the service. The socket is in a folder only root may
// write to (SocketPath), and its server must be root, so no other program
// can stand in for the service.
func Dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED):
			return nil, ErrNoService
		case errors.Is(err, fs.ErrPermission):
			return nil, ErrNotAllowed
		}
		return nil, err
	}
	p, err := peer(c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("asking who serves the TUN service socket: %w", err)
	}
	if p.uid != uint32(serverUID) {
		c.Close()
		return nil, fmt.Errorf("the TUN service socket is served by user %d, not by root", p.uid)
	}
	return c, nil
}

// Listen creates the service's socket, which every local user may open:
// the service answers root and the users with the given user ids, and
// others only CodeNotAllowed (Allowed). A socket left by a service that
// ended is replaced.
func Listen(users []string) (net.Listener, error) {
	allowed := map[uint32]bool{}
	for _, u := range users {
		uid, err := strconv.ParseUint(u, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("bad user id %q", u)
		}
		allowed[uint32(uid)] = true
	}
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o666); err != nil {
		ln.Close()
		return nil, err
	}
	return &listener{Listener: ln, allowed: allowed}, nil
}

// listener accepts every client, and notes who it is.
type listener struct {
	net.Listener
	allowed map[uint32]bool
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	p, err := peer(c)
	if err != nil {
		// Nobody known: allowed nothing.
		return &peerConn{Conn: c}, nil
	}
	return &peerConn{Conn: c, peerInfo: p, allowed: p.uid == 0 || l.allowed[p.uid]}, nil
}

// peerInfo is who is at the other end of a Unix socket.
type peerInfo struct {
	uid uint32
	pid int32
}

// peerConn is an accepted connection and who opened it.
type peerConn struct {
	net.Conn
	peerInfo
	allowed bool
}

// Allowed reports whether the client of a connection the service accepted
// may use it: root, or a user who installed it.
func Allowed(c net.Conn) bool {
	p, ok := c.(*peerConn)
	return ok && p.allowed
}

// ClientProcess is the process at the other end of a connection the
// service accepted.
func ClientProcess(c net.Conn) (uint32, error) {
	p, ok := c.(*peerConn)
	if !ok || p.pid <= 0 {
		return 0, errors.New("the client's process is not known")
	}
	return uint32(p.pid), nil
}

// Installed reports whether the service is installed: its unit or job
// file is there.
func Installed() (bool, error) {
	_, err := os.Stat(ServiceFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// fdOf runs f with the connection's file descriptor.
func fdOf(c net.Conn, f func(fd int) error) error {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return errors.New("not a socket")
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := raw.Control(func(fd uintptr) { ferr = f(int(fd)) }); err != nil {
		return err
	}
	return ferr
}
