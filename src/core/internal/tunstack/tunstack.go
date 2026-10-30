// Package tunstack runs the TUN device of the desktop TUN mode with sing-tun
// (github.com/sagernet/sing-tun): the adapter (wintun on Windows), its
// routes, and a userspace TCP/IP stack (gVisor) that hands each flow to a
// Handler. It is the only package that imports sing-tun, so that a sing-tun
// update changes nothing else; its API speaks only the standard library.
//
// The stack needs the build tag with_gvisor (cmd/firefox-vpn-tun is built
// with it); without the tag Start fails.
//
// Open creates the adapter with its addresses and DNS server but no routes;
// Start adds the routes and the stack, so that a program can first keep its
// own connections off the adapter (the core binds them to the physical
// interface) before anything is routed into it.
package tunstack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/control"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
)

// Config is the device.
type Config struct {
	// Name is the adapter's name, as the network settings show it; macOS
	// names it itself (utunN).
	Name string
	// Inet4 is the adapter's address; Inet6, when valid, its IPv6 address,
	// and IPv6 is routed into the device too.
	Inet4, Inet6 netip.Prefix
	// BlockIPv6 refuses IPv6 outside the device when it has none (Linux:
	// an unreachable rule with the routes; elsewhere the caller's filters).
	BlockIPv6 bool
	MTU       uint32
	Handler   Handler
}

// Handler receives the device's flows. UDP to port 53 (to any address) is
// DNS; other UDP goes to UDP when TakeUDP takes it, and is otherwise, like
// ICMP, refused at once with an ICMP unreachable, so apps fall back to TCP.
type Handler struct {
	// DNS answers a DNS query in wire format. It runs on the packet path
	// and must not block.
	DNS func(query []byte) ([]byte, error)
	// TCP takes a connection an app opened, in a goroutine of its own; it
	// must Accept or Refuse it.
	TCP func(f *Flow)
	// TakeUDP, when set, says whether an app's port that sends its first UDP
	// packet to destination starts a flow (whose later packets may go to
	// other destinations). It runs on the packet path, again for each new
	// destination of a port it refused, and must not block for long; what
	// it refused stays refused while packets keep coming, until ResetFlows.
	TakeUDP func(source, destination netip.AddrPort) bool
	// UDP carries a flow TakeUDP took, in a goroutine of its own, until the
	// flow ends: it must Accept or Refuse it first.
	UDP func(f *PacketFlow)
}

// PacketFlow is the UDP an app sends from one port, to any destination,
// and the replies it gets. It ends when the app sent nothing for the
// stack's UDP timeout.
type PacketFlow struct {
	Source netip.AddrPort
	conn   N.PacketConn
}

// Accept lets the flow's first packet go (the stack holds it until then).
func (f *PacketFlow) Accept() { _ = N.ReportHandshakeSuccess(f.conn) }

// Refuse answers the first packet with an ICMP unreachable and ends the
// flow.
func (f *PacketFlow) Refuse(err error) {
	_ = N.ReportHandshakeFailure(f.conn, err)
	_ = f.conn.Close()
}

// Read reads the app's next packet into b, and where it goes; a packet
// longer than b is cut.
func (f *PacketFlow) Read(b []byte) (int, netip.AddrPort, error) {
	buffer := buf.With(b)
	dst, err := f.conn.ReadPacket(buffer)
	if err != nil {
		return 0, netip.AddrPort{}, err
	}
	return buffer.Len(), dst.AddrPort(), nil
}

// Write sends b to the app as a packet from from.
func (f *PacketFlow) Write(b []byte, from netip.AddrPort) error {
	return f.conn.WritePacket(buf.As(b), M.SocksaddrFromNetIP(from))
}

// SetReadDeadline bounds the next Read.
func (f *PacketFlow) SetReadDeadline(t time.Time) error { return f.conn.SetReadDeadline(t) }

// Close ends the flow.
func (f *PacketFlow) Close() error { return f.conn.Close() }

// Flow is a TCP connection an app opened through the device. The app's
// handshake completes only when the flow is accepted, so a destination
// that cannot be reached can be refused with a reset.
type Flow struct {
	Source, Destination netip.AddrPort
	conn                net.Conn
	onClose             N.CloseHandlerFunc
	once                sync.Once
}

// Accept completes the app's handshake and returns the connection.
func (f *Flow) Accept() (net.Conn, error) {
	if hs, ok := f.conn.(N.HandshakeSuccess); ok {
		if err := hs.HandshakeSuccess(); err != nil {
			f.done(err)
			return nil, err
		}
	}
	return &flowConn{Conn: f.conn, flow: f}, nil
}

// Refuse resets the app's connection.
func (f *Flow) Refuse(err error) {
	_ = N.ReportHandshakeFailure(f.conn, err)
	_ = f.conn.Close()
	f.done(err)
}

func (f *Flow) done(err error) {
	f.once.Do(func() {
		if f.onClose != nil {
			f.onClose(err)
		}
	})
}

// flowConn tells sing-tun when an accepted flow ends.
type flowConn struct {
	net.Conn
	flow *Flow
}

func (c *flowConn) Close() error {
	err := c.Conn.Close()
	c.flow.done(nil)
	return err
}

// CloseWrite passes a half close on when the stack's connection has one.
func (c *flowConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return c.Conn.Close()
}

// Device is a TUN adapter, opened, and started once Start returned.
type Device struct {
	cfg      Config
	options  tun.Options
	tun      tun.Tun
	finder   control.InterfaceFinder
	ctx      context.Context
	cancel   context.CancelFunc
	stack    tun.Stack
	started  bool
	closeErr error
	once     sync.Once
}

// Open creates the adapter with its addresses and DNS server (the
// address after Inet4's), without routes.
func Open(cfg Config) (*Device, error) {
	if cfg.Handler.DNS == nil || cfg.Handler.TCP == nil {
		return nil, errors.New("tunstack: the handler is incomplete")
	}
	finder := control.NewDefaultInterfaceFinder()
	networks, err := tun.NewNetworkUpdateMonitor(singLogger{})
	if err != nil {
		return nil, err
	}
	monitor, err := tun.NewDefaultInterfaceMonitor(networks, singLogger{}, tun.DefaultInterfaceMonitorOptions{InterfaceFinder: finder})
	if err != nil {
		return nil, err
	}
	options := tun.Options{
		Name:             deviceName(cfg.Name),
		Inet4Address:     []netip.Prefix{cfg.Inet4},
		MTU:              cfg.MTU,
		AutoRoute:        true,
		DNSMode:          tun.DNSModeHijack,
		InterfaceFinder:  finder,
		InterfaceMonitor: monitor,
		Logger:           singLogger{},
	}
	if cfg.Inet6.IsValid() {
		options.Inet6Address = []netip.Prefix{cfg.Inet6}
	}
	platformOptions(&options, cfg)
	t, err := tun.New(options)
	if err != nil {
		return nil, fmt.Errorf("creating the TUN adapter: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Device{cfg: cfg, options: options, tun: t, finder: finder, ctx: ctx, cancel: cancel}, nil
}

// Start runs the stack on the device and routes everything into it.
func (d *Device) Start() error {
	if d.started {
		return errors.New("tunstack: already started")
	}
	s, err := tun.NewStack("gvisor", tun.StackOptions{
		Context:    d.ctx,
		Tun:        d.tun,
		TunOptions: d.options,
		// A UDP flow that TakeUDP took ends this long after its app's last
		// packet.
		UDPTimeout:      time.Minute,
		Handler:         &handler{h: d.cfg.Handler},
		Logger:          singLogger{},
		InterfaceFinder: d.finder,
	})
	if err != nil {
		return fmt.Errorf("creating the TCP/IP stack: %w", err)
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("starting the TCP/IP stack: %w", err)
	}
	d.stack = s
	if err := d.tun.Start(); err != nil {
		return fmt.Errorf("adding the routes: %w", err)
	}
	d.started = true
	return nil
}

// ResetFlows ends the UDP flows and forgets what was judged, so that each
// flow's next packet is judged again (TakeUDP changed its mind); TCP
// connections go on.
func (d *Device) ResetFlows() {
	if d.stack != nil {
		d.stack.ResetNetwork()
	}
}

// Close ends every flow, the stack, and the adapter with its routes.
func (d *Device) Close() error {
	d.once.Do(func() {
		d.cancel()
		if d.stack != nil {
			_ = d.stack.Close()
		}
		d.closeErr = d.tun.Close()
	})
	return d.closeErr
}

// handler adapts Handler to sing-tun's.
type handler struct{ h Handler }

const (
	protocolTCP = 6
	protocolUDP = 17
)

func (h *handler) JudgeFlow(network uint8, source, destination netip.AddrPort, _ []byte) tun.FlowVerdict {
	switch {
	case network == protocolTCP:
		return tun.FlowVerdict{Action: tun.ActionAccept}
	case network == protocolUDP && destination.Port() == 53:
		return tun.FlowVerdict{Action: tun.ActionHijackDNS}
	case network == protocolUDP && h.h.TakeUDP != nil && h.h.UDP != nil && h.h.TakeUDP(source, destination):
		return tun.FlowVerdict{Action: tun.ActionAccept}
	}
	return tun.FlowVerdict{Action: tun.ActionReject}
}

func (h *handler) NewDNSPacket(payload []byte, _, destination M.Socksaddr, writer N.PacketWriter) {
	reply, err := h.h.DNS(payload)
	if err != nil {
		logx.Debug("TUN: a DNS query was not answered err=%s", logx.Err(err))
		return
	}
	if err := writer.WritePacket(buf.As(reply), destination); err != nil {
		logx.Debug("TUN: sending a DNS answer failed err=%s", logx.Err(err))
	}
}

func (h *handler) NewConnectionEx(_ context.Context, conn net.Conn, source, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	h.h.TCP(&Flow{Source: source.AddrPort(), Destination: destination.AddrPort(), conn: conn, onClose: onClose})
}

// NewPacketConnectionEx gets the UDP flows JudgeFlow took.
func (h *handler) NewPacketConnectionEx(_ context.Context, conn N.PacketConn, source, _ M.Socksaddr, onClose N.CloseHandlerFunc) {
	if h.h.UDP != nil {
		h.h.UDP(&PacketFlow{Source: source.AddrPort(), conn: conn})
	}
	_ = conn.Close()
	if onClose != nil {
		onClose(nil)
	}
}

// singLogger sends sing-tun's log to the program's: debug lines when
// verbose, the rest as warnings and errors.
type singLogger struct{}

func (singLogger) Trace(...any)      {}
func (singLogger) Debug(args ...any) { logx.Debug("TUN: %s", fmt.Sprint(args...)) }
func (singLogger) Info(args ...any)  { logx.Debug("TUN: %s", fmt.Sprint(args...)) }
func (singLogger) Warn(args ...any)  { logx.Warn("TUN: %s", fmt.Sprint(args...)) }
func (singLogger) Error(args ...any) { logx.Error("TUN: %s", fmt.Sprint(args...)) }
func (singLogger) Fatal(args ...any) { logx.Error("TUN: %s", fmt.Sprint(args...)) }
func (singLogger) Panic(args ...any) { logx.Error("TUN: %s", fmt.Sprint(args...)) }
