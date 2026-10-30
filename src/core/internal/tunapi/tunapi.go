// Package tunapi is the local protocol between the core and the TUN service
// (cmd/firefox-vpn-tun), which holds the TUN device of the desktop TUN mode
// with the system's rights. Messages are JSON objects, one per line, over a
// channel that only the users who installed the service can use: a named
// pipe on Windows (pipe_windows.go), whose access list keeps others out; a
// Unix socket on Linux and macOS (socket_unix.go), where the service asks
// the kernel who connected and answers others only CodeNotAllowed.
//
// A client says hello first, and both sides learn each other's version. The
// core then opens a TUN session (open: the adapter without routes; route:
// the routes, the stack, and the DNS rules) and keeps the connection for as
// long as the TUN should run: when the connection ends, for whatever reason,
// the service takes the TUN down. update and uninstall need no session and
// keep their meaning in every protocol version, so any app can replace or
// remove any installed service.
//
// Protocol 2 added the programs that skip the VPN (open's Programs, and
// programs to change them while the TUN runs) and udp_refused.
package tunapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

// Protocol is the protocol version of this build; MinProtocol is the oldest
// it still speaks. A change that an older peer would misread raises
// Protocol; dropping support for an old peer raises MinProtocol.
const (
	Protocol    = 2
	MinProtocol = 1
)

// Message types. A request is answered by its reply type or by TypeError;
// TypeFailed comes unasked, when the TUN stops by itself, and
// TypeUDPRefused while it runs.
const (
	TypeHello        = "hello"
	TypeOpen         = "open"
	TypeOpened       = "opened"
	TypeRoute        = "route"
	TypeRouted       = "routed"
	TypeClose        = "close"
	TypeClosed       = "closed"
	TypeFailed       = "failed"
	TypeError        = "error"
	TypeUpdate       = "update"
	TypeUpdating     = "updating"
	TypeUninstall    = "uninstall"
	TypeUninstalling = "uninstalling"
	// TypePrograms (protocol 2) replaces the session's programs that skip
	// the VPN; TypeProgramsSet answers it.
	TypePrograms    = "programs"
	TypeProgramsSet = "programs_set"
	// TypeUDPRefused (protocol 2): a program's UDP that looks like a call or
	// a game was refused; once per program in a session.
	TypeUDPRefused = "udp_refused"
)

// Error codes (Message.Code of TypeError and TypeFailed).
const (
	// CodeProtocol: no protocol version both sides speak; the reply carries
	// the service's protocol range and version.
	CodeProtocol = "protocol"
	// CodeBusy: another client's TUN session runs.
	CodeBusy = "busy"
	// CodeBadRequest: an unknown or misplaced message.
	CodeBadRequest = "bad_request"
	// CodeOpenFailed: the TUN adapter could not be created.
	CodeOpenFailed = "open_failed"
	// CodeRouteFailed: the routes, the stack, or the DNS rules failed.
	CodeRouteFailed = "route_failed"
	// CodeAdapterLost: the adapter went away while the TUN ran (disabled in
	// the network settings, or removed).
	CodeAdapterLost = "adapter_lost"
	// CodeUpdateRefused: the update's file is not signed, not newer, or
	// unreadable.
	CodeUpdateRefused = "update_refused"
	// CodeUninstallFailed: the service could not remove itself.
	CodeUninstallFailed = "uninstall_failed"
	// CodeNotAllowed: the user who connected did not install the service
	// (the reply to hello on a Unix socket; on Windows the pipe's access
	// list keeps them out). Installing it as that user lets them in.
	CodeNotAllowed = "not_allowed"
)

// Message is every message; each type uses some of the fields.
type Message struct {
	Type string `json:"type"`
	// Protocol and MinProtocol (hello, and an error with CodeProtocol) are
	// the sender's protocol range; Version is its program version.
	Protocol    int    `json:"protocol,omitempty"`
	MinProtocol int    `json:"min_protocol,omitempty"`
	Version     string `json:"version,omitempty"`
	// Proxy, Username, and Password (open) are the SOCKS5 server the TUN
	// sends its connections to, and its login.
	Proxy    string `json:"proxy,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// Interface (opened) is the TUN adapter.
	Interface *Interface `json:"interface,omitempty"`
	// Path (update) is the new service executable; its signature is in
	// Path + ".sig". Path (udp_refused) is the program's executable.
	Path string `json:"path,omitempty"`
	// Programs (open, programs) skip the VPN: the service sends their
	// traffic directly, outside the TUN, instead of to the core.
	Programs []Program `json:"programs,omitempty"`
	// Program (udp_refused) is the program's executable file name.
	Program string `json:"program,omitempty"`
	// Code and Message (error, failed) say why.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Program is a program that skips the VPN, by its executable's file name
// (steam.exe), compared without case.
type Program struct {
	Name string `json:"name"`
	// UDPOnly: only its UDP (calls, games) skips the VPN; its TCP still goes
	// to the core.
	UDPOnly bool `json:"udp_only,omitempty"`
}

// Interface identifies the TUN adapter, so the core can keep its own
// connections off it.
type Interface struct {
	Name  string `json:"name"`
	Index uint32 `json:"index"`
	LUID  uint64 `json:"luid"`
}

// MaxMessage bounds one message line.
const MaxMessage = 64 << 10

// ErrTooLong is a message line longer than MaxMessage.
var ErrTooLong = errors.New("tunapi: message too long")

// Conn sends and receives messages on a connection. Send is safe for
// concurrent use; Receive is not.
type Conn struct {
	raw net.Conn
	r   *bufio.Reader
	wmu sync.Mutex
}

// NewConn wraps c.
func NewConn(c net.Conn) *Conn {
	return &Conn{raw: c, r: bufio.NewReaderSize(c, 4096)}
}

// Raw is the underlying connection.
func (c *Conn) Raw() net.Conn { return c.raw }

// Close closes the connection.
func (c *Conn) Close() error { return c.raw.Close() }

// Send writes m as one line.
func (c *Conn) Send(m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) >= MaxMessage {
		return ErrTooLong
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = c.raw.Write(append(data, '\n'))
	return err
}

// Receive reads the next message; io.EOF when the peer closed the
// connection between messages.
func (c *Conn) Receive() (Message, error) {
	var line []byte
	for {
		chunk, err := c.r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxMessage {
			return Message{}, ErrTooLong
		}
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return Message{}, io.ErrUnexpectedEOF
		}
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return Message{}, fmt.Errorf("tunapi: bad message: %w", err)
	}
	if m.Type == "" {
		return Message{}, errors.New("tunapi: message without a type")
	}
	return m, nil
}

// Common is the newest protocol version this build and a peer speaking
// minProtocol..protocol both speak, or 0 when there is none.
func Common(protocol, minProtocol int) int {
	p := min(Protocol, protocol)
	if p < max(MinProtocol, minProtocol) {
		return 0
	}
	return p
}

// Hello is this build's hello message.
func Hello(version string) Message {
	return Message{Type: TypeHello, Protocol: Protocol, MinProtocol: MinProtocol, Version: version}
}

// Errorf builds an error reply.
func Errorf(code, format string, args ...any) Message {
	return Message{Type: TypeError, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Error is a TypeError or TypeFailed message as a Go error.
type Error struct {
	Code    string
	Message string
	// Protocol, MinProtocol, and Version are the peer's (CodeProtocol).
	Protocol    int
	MinProtocol int
	Version     string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return "TUN service: " + e.Code
	}
	return "TUN service: " + e.Message
}

// AsError returns m as an *Error when it is an error or a failure, else
// nil.
func AsError(m Message) error {
	if m.Type != TypeError && m.Type != TypeFailed {
		return nil
	}
	return &Error{Code: m.Code, Message: m.Message, Protocol: m.Protocol, MinProtocol: m.MinProtocol, Version: m.Version}
}
