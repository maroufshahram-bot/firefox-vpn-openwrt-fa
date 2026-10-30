// Package tunservice is the TUN service (cmd/firefox-vpn-tun): it serves
// the tunapi protocol on the service's channel and runs one TUN session at
// a time, for the core that opened it, until that core closes it or its
// connection ends. Every TCP flow goes to the core's SOCKS5 server on
// loopback, but those of the programs that skip the VPN (programs.go):
// their traffic is the only one the service sends to the internet itself,
// directly, outside the TUN.
package tunservice

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tun"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunstack"
)

// adapterMTU is the TUN adapter's MTU; AdapterName (adapter_*.go) its name.
const adapterMTU = 9000

var (
	// adapterV4 is the adapter's address in tun.DesktopNet; Windows sends
	// DNS to the next one, which the stack answers like any DNS.
	adapterV4 = netip.MustParsePrefix("198.18.0.1/30")
	// adapterV6 routes IPv6 into the adapter too, so that it never leaves
	// outside the VPN.
	adapterV6 = netip.MustParsePrefix("fdfe:dcba:9876::1/126")
)

// Config is what the service does beyond the protocol.
type Config struct {
	// Version is the service's program version.
	Version string
	// Self is the service's executable: the filters let it through.
	Self string
	// Update checks the executable at path and makes it the service, to
	// run once this process ends; nil refuses updates.
	Update func(path string) error
	// Uninstall removes the service, which ends once Serve returns; nil
	// refuses.
	Uninstall func() error
}

// device is the part of tunstack.Device the server uses; tests fake it.
type device interface {
	Interface() (name string, index uint32, luid uint64, err error)
	Start() error
	ResetFlows()
	Close() error
}

// Server serves the protocol.
type Server struct {
	cfg Config
	// Replaced in tests.
	openDevice   func(tunstack.Config) (device, error)
	startFilters func(index uint32, programs []string, blockIPv6 bool) (io.Closer, error)
	adapterUp    func(index uint32) bool
	clientPath   func(net.Conn) (string, error)
	// allowed reports whether a client may use the service (a Unix
	// socket's peer; on Windows the pipe's access list decided).
	allowed func(net.Conn) bool

	mu       sync.Mutex
	session  *session
	closing  bool
	listener net.Listener
	conns    map[*tunapi.Conn]struct{}
	// ended is closed once an update or an uninstall asks the process to
	// end; reason says which.
	ended  chan struct{}
	reason string
}

// NewServer returns a server for cfg.
func NewServer(cfg Config) *Server {
	return &Server{
		cfg: cfg,
		openDevice: func(c tunstack.Config) (device, error) {
			return tunstack.Open(c)
		},
		startFilters: startFilters,
		adapterUp:    adapterUp,
		allowed:      tunapi.Allowed,
		clientPath: func(c net.Conn) (string, error) {
			pid, err := tunapi.ClientProcess(c)
			if err != nil {
				return "", err
			}
			return tunapi.ProcessPath(pid)
		},
		conns: map[*tunapi.Conn]struct{}{},
		ended: make(chan struct{}),
	}
}

// Ended is closed when an update or an uninstall wants the process to
// end; Reason then says which.
func (s *Server) Ended() <-chan struct{} { return s.ended }

// Reason is why the server ended: "updated" or "uninstalled".
func (s *Server) Reason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// Serve accepts clients on ln until Close.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		ln.Close()
		return net.ErrClosed
	}
	s.listener = ln
	s.mu.Unlock()
	for {
		raw, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.serveConn(raw)
	}
}

// Close ends the session, the connections, and Serve.
func (s *Server) Close() {
	s.mu.Lock()
	s.closing = true
	ln := s.listener
	sess := s.session
	s.session = nil
	conns := make([]*tunapi.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
	if sess != nil {
		sess.close()
	}
	for _, c := range conns {
		c.Close()
	}
}

// end closes the server and tells the process to end.
func (s *Server) end(reason string) {
	s.mu.Lock()
	first := s.reason == ""
	if first {
		s.reason = reason
	}
	s.mu.Unlock()
	if !first {
		return
	}
	s.Close()
	close(s.ended)
}

func (s *Server) serveConn(raw net.Conn) {
	c := tunapi.NewConn(raw)
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		c.Close()
		return
	}
	s.conns[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		// A session ends with its core's connection, whatever the reason.
		s.closeSession(c)
		c.Close()
	}()

	hello, err := c.Receive()
	if err != nil {
		return
	}
	if hello.Type != tunapi.TypeHello {
		_ = c.Send(tunapi.Errorf(tunapi.CodeBadRequest, "say hello first"))
		return
	}
	// Nothing for a user who did not install the service, whatever the
	// protocol (update and uninstall work in every one); told why, so the
	// app installs it for this user.
	if !s.allowed(raw) {
		logx.Warn("a user who did not install the service connected")
		_ = c.Send(tunapi.Errorf(tunapi.CodeNotAllowed, "the TUN service was installed for another user"))
		return
	}
	protocol := tunapi.Common(hello.Protocol, hello.MinProtocol)
	compatible := protocol != 0
	reply := tunapi.Hello(s.cfg.Version)
	if !compatible {
		reply = tunapi.Errorf(tunapi.CodeProtocol, "no common protocol: the service speaks %d to %d, the client %d to %d", tunapi.MinProtocol, tunapi.Protocol, hello.MinProtocol, hello.Protocol)
		reply.Protocol, reply.MinProtocol, reply.Version = tunapi.Protocol, tunapi.MinProtocol, s.cfg.Version
		logx.Warn("a client of version %s speaks no common protocol", hello.Version)
	}
	if err := c.Send(reply); err != nil {
		return
	}
	for {
		m, err := c.Receive()
		if err != nil {
			return
		}
		var out tunapi.Message
		switch m.Type {
		case tunapi.TypeUpdate:
			out = s.update(m.Path)
		case tunapi.TypeUninstall:
			out = s.uninstall()
		case tunapi.TypeOpen, tunapi.TypeRoute, tunapi.TypeClose, tunapi.TypePrograms:
			if !compatible {
				out = tunapi.Errorf(tunapi.CodeProtocol, "no common protocol")
				break
			}
			switch m.Type {
			case tunapi.TypeOpen:
				out = s.open(c, m, protocol)
			case tunapi.TypeRoute:
				out = s.route(c)
			case tunapi.TypePrograms:
				out = s.programs(c, m.Programs)
			default:
				s.closeSession(c)
				out = tunapi.Message{Type: tunapi.TypeClosed}
			}
		default:
			out = tunapi.Errorf(tunapi.CodeBadRequest, "unknown message %q", m.Type)
		}
		if err := c.Send(out); err != nil {
			return
		}
		if out.Type == tunapi.TypeRouted {
			// From now on the core reads what comes unasked.
			if sess := s.sessionOf(c); sess != nil {
				sess.live.Store(true)
			}
		}
		if out.Type == tunapi.TypeUpdating {
			s.end("updated")
			return
		}
		if out.Type == tunapi.TypeUninstalling {
			s.end("uninstalled")
			return
		}
	}
}

func (s *Server) open(c *tunapi.Conn, m tunapi.Message, protocol int) tunapi.Message {
	// The service connects nowhere but to the core on this computer.
	if proxy, err := netip.ParseAddrPort(m.Proxy); err != nil || !proxy.Addr().IsLoopback() {
		return tunapi.Errorf(tunapi.CodeBadRequest, "the proxy must be a loopback address and port, not %q", m.Proxy)
	}
	client, err := s.clientPath(c.Raw())
	if err != nil {
		return tunapi.Errorf(tunapi.CodeOpenFailed, "finding the client's program: %s", err)
	}
	s.mu.Lock()
	if s.session != nil {
		owner := s.session.owner
		s.mu.Unlock()
		if owner != c {
			return tunapi.Errorf(tunapi.CodeBusy, "another program's TUN session runs")
		}
		return tunapi.Errorf(tunapi.CodeBadRequest, "the TUN session is already open")
	}
	// Taken now, so a second client meanwhile gets busy.
	placeholder := &session{owner: c}
	s.session = placeholder
	s.mu.Unlock()

	sess, err := newSession(s, c, client, protocol, m)
	s.mu.Lock()
	if s.session == placeholder {
		s.session = sess
	}
	current := s.session == sess
	s.mu.Unlock()
	if err != nil {
		s.mu.Lock()
		if s.session == placeholder {
			s.session = nil
		}
		s.mu.Unlock()
		logx.Error("opening the TUN adapter failed err=%s", logx.Err(err))
		return tunapi.Errorf(tunapi.CodeOpenFailed, "%s", err)
	}
	if !current {
		// The server closed meanwhile.
		sess.close()
		return tunapi.Errorf(tunapi.CodeOpenFailed, "the service is stopping")
	}
	name, index, luid := sess.iface()
	logx.Info("TUN adapter opened name=%q index=%d client=%s", name, index, client)
	return tunapi.Message{Type: tunapi.TypeOpened, Interface: &tunapi.Interface{Name: name, Index: index, LUID: luid}}
}

func (s *Server) route(c *tunapi.Conn) tunapi.Message {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil || sess.owner != c || sess.dev == nil {
		return tunapi.Errorf(tunapi.CodeBadRequest, "open the TUN session first")
	}
	if err := sess.route(); err != nil {
		s.closeSession(c)
		logx.Error("starting the TUN failed err=%s", logx.Err(err))
		return tunapi.Errorf(tunapi.CodeRouteFailed, "%s", err)
	}
	logx.Info("TUN started: all traffic goes to the core")
	return tunapi.Message{Type: tunapi.TypeRouted}
}

// programs replaces the programs that skip the VPN in c's session.
func (s *Server) programs(c *tunapi.Conn, programs []tunapi.Program) tunapi.Message {
	sess := s.sessionOf(c)
	if sess == nil || sess.dev == nil {
		return tunapi.Errorf(tunapi.CodeBadRequest, "open the TUN session first")
	}
	sess.setPrograms(programs)
	return tunapi.Message{Type: tunapi.TypeProgramsSet}
}

// sessionOf is c's session, or nil.
func (s *Server) sessionOf(c *tunapi.Conn) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.owner != c {
		return nil
	}
	return s.session
}

// closeSession ends c's session, if it has one.
func (s *Server) closeSession(c *tunapi.Conn) {
	s.mu.Lock()
	sess := s.session
	if sess == nil || sess.owner != c {
		s.mu.Unlock()
		return
	}
	s.session = nil
	s.mu.Unlock()
	sess.close()
}

// lost ends sess after its adapter went away, telling its core.
func (s *Server) lost(sess *session) {
	s.mu.Lock()
	if s.session != sess {
		s.mu.Unlock()
		return
	}
	s.session = nil
	s.mu.Unlock()
	logx.Warn("the TUN adapter went away; the TUN is stopped")
	sess.close()
	_ = sess.owner.Send(tunapi.Message{Type: tunapi.TypeFailed, Code: tunapi.CodeAdapterLost, Message: "the TUN adapter was disabled or removed"})
}

func (s *Server) update(path string) tunapi.Message {
	if s.cfg.Update == nil {
		return tunapi.Errorf(tunapi.CodeUpdateRefused, "this service takes no updates")
	}
	if err := s.cfg.Update(path); err != nil {
		logx.Warn("an update was refused path=%s err=%s", path, logx.Err(err))
		return tunapi.Errorf(tunapi.CodeUpdateRefused, "%s", err)
	}
	logx.Info("updated from path=%s; restarting", path)
	return tunapi.Message{Type: tunapi.TypeUpdating}
}

func (s *Server) uninstall() tunapi.Message {
	if s.cfg.Uninstall == nil {
		return tunapi.Errorf(tunapi.CodeUninstallFailed, "this service cannot remove itself")
	}
	s.mu.Lock()
	sess := s.session
	s.session = nil
	s.mu.Unlock()
	if sess != nil {
		sess.close()
	}
	if err := s.cfg.Uninstall(); err != nil {
		logx.Error("removing the service failed err=%s", logx.Err(err))
		return tunapi.Errorf(tunapi.CodeUninstallFailed, "%s", err)
	}
	logx.Info("the service is removed")
	return tunapi.Message{Type: tunapi.TypeUninstalling}
}

// isFilterAddress is tun.IsFilterAddress, for the session.
var isFilterAddress = tun.IsFilterAddress

// adapterUp reports whether the interface index is there and up.
func adapterUp(index uint32) bool {
	iface, err := net.InterfaceByIndex(int(index))
	return err == nil && iface.Flags&net.FlagUp != 0
}
