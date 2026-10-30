//go:build !windows && !linux && !darwin

package tunapi

import (
	"context"
	"errors"
	"net"
)

// ServiceName and ExeName name the service and its executable.
const (
	ServiceName = "firefox-vpn-tun"
	ExeName     = "firefox-vpn-tun"
)

// ErrNoService means no service answers: it is not installed, or it is
// not running.
var ErrNoService = errors.New("the TUN service is not running")

// ErrNotAllowed means the service runs but this user did not install it.
var ErrNotAllowed = errors.New("the TUN service was installed for another user")

var errUnsupported = errors.New("the TUN mode is not available on this system yet")

// Dial connects to the service.
func Dial(context.Context) (net.Conn, error) { return nil, ErrNoService }

// Listen creates the service's channel.
func Listen([]string) (net.Listener, error) { return nil, errUnsupported }

// Installed reports whether the service is installed.
func Installed() (bool, error) { return false, nil }

// ClientProcess is the process at the other end of a connection the
// service accepted.
func ClientProcess(net.Conn) (uint32, error) { return 0, errUnsupported }

// ProcessPath is the executable of process pid.
func ProcessPath(uint32) (string, error) { return "", errUnsupported }

// Allowed reports whether the client of a connection the service accepted
// may use it.
func Allowed(net.Conn) bool { return false }
