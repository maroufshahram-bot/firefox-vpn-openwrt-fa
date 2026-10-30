package tunservice

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/egress"
	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tun"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunstack"
)

// session is one core's TUN: the adapter, then its routes and stack and
// the DNS filters.
type session struct {
	owner   *tunapi.Conn
	server  *Server
	dev     device
	filters io.Closer
	dns     *tun.FakeDNS
	dial    tun.Dialer
	// protocol is the protocol version the core speaks.
	protocol int
	// trusted pass the DNS filters: the service and the core.
	trusted []string
	// programs skip the VPN (programs.go); owners keeps UDP ports' ones.
	programs atomic.Pointer[programSet]
	owners   owners
	// live: the core was told the TUN is routed, so it reads what comes
	// unasked; told holds the programs whose refused UDP it was told of.
	live   atomic.Bool
	toldMu sync.Mutex
	told   map[string]bool
	// noIPv6: the adapter has no IPv6 (the computer turned it off), so the
	// filters block IPv6 outside it.
	noIPv6 bool
	name   string
	index  uint32
	luid   uint64
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// dialTimeout bounds a connection through the core, which may first wake
// and connect to Mozilla.
const dialTimeout = 30 * time.Second

// dnsIdle ends a DNS-over-TCP connection that long after it opened.
const dnsIdle = 10 * time.Second

// adapterCheck is how often the adapter is looked for while the TUN runs.
var adapterCheck = 2 * time.Second

// The service's own sockets for the programs that skip the VPN leave
// outside the adapter, as the core's do; replaced in tests.
var (
	setEgress   = egress.SetTun
	clearEgress = egress.ClearTun
	// dialDirect connects a program that skips the VPN directly, a name
	// through the physical network's DNS.
	dialDirect tun.Dialer = func(ctx context.Context, target string) (net.Conn, error) {
		return egress.ClientDialer(0).DialContext(ctx, "tcp", target)
	}
)

// Errors that refuse a flow; they are only logged.
var (
	errFilterAddress = errors.New("an address of the filtering DNS")
	errForgotten     = errors.New("a fake address the DNS no longer knows")
)

// newSession opens the adapter for the core client, speaking protocol,
// whose open message m names its SOCKS5 server and the programs that skip
// the VPN.
func newSession(s *Server, owner *tunapi.Conn, client string, protocol int, m tunapi.Message) (*session, error) {
	dial, err := tun.SOCKS5Dialer(m.Proxy, m.Username, m.Password)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	sess := &session{
		owner:    owner,
		server:   s,
		dns:      tun.NewDesktopFakeDNS(),
		dial:     dial,
		protocol: protocol,
		trusted:  []string{s.cfg.Self, client},
		ctx:      ctx,
		cancel:   cancel,
	}
	sess.setPrograms(m.Programs)
	cfg := tunstack.Config{
		Name:  AdapterName,
		Inet4: adapterV4,
		Inet6: adapterV6,
		MTU:   adapterMTU,
		Handler: tunstack.Handler{
			DNS:     sess.dns.Answer,
			TCP:     func(f *tunstack.Flow) { sess.forward(f.Source, f.Destination, f.Accept, f.Refuse) },
			TakeUDP: sess.takeUDP,
			UDP:     sess.relayUDP,
		},
	}
	dev, err := s.openDevice(cfg)
	if err != nil {
		// IPv6 may be turned off on the computer (DisabledComponents), and
		// the adapter then gets no IPv6 address.
		logx.Warn("opening the TUN adapter with IPv6 failed; opening it without, with IPv6 blocked err=%s", logx.Err(err))
		cfg.Inet6 = netip.Prefix{}
		cfg.BlockIPv6 = true
		sess.noIPv6 = true
		dev, err = s.openDevice(cfg)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	sess.dev = dev
	if sess.name, sess.index, sess.luid, err = dev.Interface(); err != nil {
		_ = dev.Close()
		cancel()
		return nil, err
	}
	setEgress(sess.index, sess.luid)
	return sess, nil
}

func (s *session) iface() (string, uint32, uint64) { return s.name, s.index, s.luid }

// route starts the stack and the routes, then the DNS filters, and
// watches the adapter.
func (s *session) route() error {
	if err := s.dev.Start(); err != nil {
		return err
	}
	filters, err := s.server.startFilters(s.index, s.trusted, s.noIPv6)
	if err != nil {
		return err
	}
	s.filters = filters
	go s.watch()
	return nil
}

// watch ends the session when the adapter goes away (disabled in the
// network settings, or removed), so the core can tell the user.
func (s *session) watch() {
	t := time.NewTicker(adapterCheck)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			if !s.server.adapterUp(s.index) {
				s.server.lost(s)
				return
			}
		}
	}
}

// close removes the routes and the adapter first, then the filters, so
// DNS never leaves while the adapter still carries the traffic.
func (s *session) close() {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.dev != nil {
			if err := s.dev.Close(); err != nil {
				logx.Warn("closing the TUN adapter failed err=%s", logx.Err(err))
			}
			clearEgress()
		}
		if s.filters != nil {
			if err := s.filters.Close(); err != nil {
				logx.Warn("removing the DNS filters failed err=%s", logx.Err(err))
			}
		}
		if s.dev != nil {
			logx.Info("TUN stopped")
		}
	})
}

// forward carries an app's connection from src to dst: DNS over TCP is
// answered here; a connection to a fake address goes to its name (to a
// real address, to that address), through the core, or directly when its
// program skips the VPN; the app's handshake completes only once that
// connected, so a failure resets it at once.
func (s *session) forward(src, dst netip.AddrPort, accept func() (net.Conn, error), refuse func(error)) {
	if dst.Port() == 53 {
		conn, err := accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(dnsIdle))
		_ = s.dns.ServeStream(conn)
		return
	}
	if isFilterAddress(dst.Addr()) {
		// A filtering DNS's answer that an app kept from before the TUN
		// leads nowhere: refused, so the app asks the DNS again.
		refuse(errFilterAddress)
		return
	}
	target, ok := s.dns.Target(dst)
	if !ok {
		refuse(errForgotten)
		return
	}
	dial := s.dial
	if s.directTCP(src) {
		dial = dialDirect
	}
	ctx, cancel := context.WithTimeout(s.ctx, dialTimeout)
	upstream, err := dial(ctx, target)
	cancel()
	if err != nil {
		refuse(err)
		if s.ctx.Err() == nil {
			logx.Debug("TUN connection failed target=%s err=%s", logx.Target(target), logx.Err(err))
		}
		return
	}
	conn, err := accept()
	if err != nil {
		_ = upstream.Close()
		return
	}
	tun.Relay(conn, upstream)
}
