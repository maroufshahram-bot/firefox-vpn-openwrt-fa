package tunservice

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/tun"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunstack"
)

// fakeDevice stands for the adapter.
type fakeDevice struct {
	started, closed atomic.Bool
	resets          atomic.Int32
	startErr        error
}

func (d *fakeDevice) ResetFlows() { d.resets.Add(1) }

func (d *fakeDevice) Interface() (string, uint32, uint64, error) {
	return AdapterName, 42, 7 << 32, nil
}
func (d *fakeDevice) Start() error {
	if d.startErr != nil {
		return d.startErr
	}
	d.started.Store(true)
	return nil
}
func (d *fakeDevice) Close() error { d.closed.Store(true); return nil }

type fakeFilters struct {
	mu        sync.Mutex
	index     uint32
	programs  []string
	blockIPv6 bool
	closed    bool
}

func (f *fakeFilters) Close() error { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }

// harness is a server with fakes, and what it did.
type harness struct {
	s       *Server
	devices chan *fakeDevice
	filters chan *fakeFilters
	up      atomic.Bool
	// noIPv6 makes an adapter with IPv6 fail to open, as on a computer
	// with IPv6 turned off.
	noIPv6 atomic.Bool
	// stranger: the clients are a user who did not install the service.
	stranger atomic.Bool
	ln       *pipeListener
}

func newHarness(t *testing.T, cfg Config) *harness {
	t.Helper()
	h := &harness{devices: make(chan *fakeDevice, 4), filters: make(chan *fakeFilters, 4)}
	h.up.Store(true)
	// The test's own sockets stay as they are.
	oldSet, oldClear := setEgress, clearEgress
	setEgress, clearEgress = func(uint32, uint64) {}, func() {}
	t.Cleanup(func() { setEgress, clearEgress = oldSet, oldClear })
	if cfg.Version == "" {
		cfg.Version = "1.2.3"
	}
	cfg.Self = `C:\Program Files\Firefox VPN\firefox-vpn-tun.exe`
	h.s = NewServer(cfg)
	h.s.openDevice = func(c tunstack.Config) (device, error) {
		if h.noIPv6.Load() && c.Inet6.IsValid() {
			return nil, errors.New("set ipv6 address: Element not found.")
		}
		d := &fakeDevice{}
		h.devices <- d
		return d, nil
	}
	h.s.startFilters = func(index uint32, programs []string, blockIPv6 bool) (io.Closer, error) {
		f := &fakeFilters{index: index, programs: programs, blockIPv6: blockIPv6}
		h.filters <- f
		return f, nil
	}
	h.s.adapterUp = func(uint32) bool { return h.up.Load() }
	h.s.clientPath = func(net.Conn) (string, error) { return `C:\Apps\Firefox VPN\core\firefox-vpn-core.exe`, nil }
	h.s.allowed = func(c net.Conn) bool { return !h.stranger.Load() }
	h.ln = newPipeListener()
	go func() { _ = h.s.Serve(h.ln) }()
	t.Cleanup(h.s.Close)
	return h
}

// pipeListener hands out in-memory connections.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *pipeListener) Addr() net.Addr { return &net.UnixAddr{Name: "test", Net: "pipe"} }

func (h *harness) dial(t *testing.T) *tunapi.Conn {
	t.Helper()
	a, b := net.Pipe()
	select {
	case h.ln.conns <- b:
	case <-time.After(5 * time.Second):
		t.Fatal("the server does not accept")
	}
	c := tunapi.NewConn(a)
	t.Cleanup(func() { c.Close() })
	return c
}

func roundTrip(t *testing.T, c *tunapi.Conn, m tunapi.Message) tunapi.Message {
	t.Helper()
	if err := c.Send(m); err != nil {
		t.Fatal(err)
	}
	reply, err := c.Receive()
	if err != nil {
		t.Fatalf("no reply to %s: %v", m.Type, err)
	}
	return reply
}

func hello(t *testing.T, c *tunapi.Conn) {
	t.Helper()
	if reply := roundTrip(t, c, tunapi.Hello("9.9.9")); reply.Type != tunapi.TypeHello || reply.Version != "1.2.3" {
		t.Fatalf("hello: %+v", reply)
	}
}

var openMessage = tunapi.Message{Type: tunapi.TypeOpen, Proxy: "127.0.0.1:5000", Username: "u", Password: "p"}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestATUNSessionOpensRoutesAndCloses(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	opened := roundTrip(t, c, openMessage)
	if opened.Type != tunapi.TypeOpened || opened.Interface == nil || opened.Interface.Index != 42 || opened.Interface.Name != AdapterName {
		t.Fatalf("open: %+v", opened)
	}
	dev := <-h.devices
	if dev.started.Load() {
		t.Fatal("the stack and routes started before route")
	}
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute}); reply.Type != tunapi.TypeRouted {
		t.Fatalf("route: %+v", reply)
	}
	f := <-h.filters
	if !dev.started.Load() || f.index != 42 || len(f.programs) != 2 ||
		!strings.HasSuffix(f.programs[0], "firefox-vpn-tun.exe") || !strings.HasSuffix(f.programs[1], "firefox-vpn-core.exe") {
		t.Fatalf("started %v, filters %+v", dev.started.Load(), f)
	}
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeClose}); reply.Type != tunapi.TypeClosed {
		t.Fatalf("close: %+v", reply)
	}
	if !dev.closed.Load() || !f.closed {
		t.Fatal("closing left the adapter or the filters")
	}
}

// On a computer with IPv6 turned off the adapter opens without it, and
// the filters block IPv6, which then has no route into the adapter.
func TestWithoutIPv6TheAdapterOpensAndIPv6IsBlocked(t *testing.T) {
	h := newHarness(t, Config{})
	h.noIPv6.Store(true)
	c := h.dial(t)
	hello(t, c)
	if reply := roundTrip(t, c, openMessage); reply.Type != tunapi.TypeOpened {
		t.Fatalf("open: %+v", reply)
	}
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute}); reply.Type != tunapi.TypeRouted {
		t.Fatalf("route: %+v", reply)
	}
	if f := <-h.filters; !f.blockIPv6 {
		t.Fatal("IPv6 is not blocked")
	}
	roundTrip(t, c, tunapi.Message{Type: tunapi.TypeClose})

	// With IPv6, it goes into the adapter and is not blocked.
	h.noIPv6.Store(false)
	roundTrip(t, c, openMessage)
	roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute})
	if f := <-h.filters; f.blockIPv6 {
		t.Fatal("IPv6 is blocked with an IPv6 adapter")
	}
}

// The TUN ends with its core: a connection that ends (the core quit or
// crashed) takes everything down.
func TestTheTUNEndsWithTheCoresConnection(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	roundTrip(t, c, openMessage)
	roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute})
	dev, f := <-h.devices, <-h.filters
	c.Close()
	waitFor(t, "the adapter to close", dev.closed.Load)
	waitFor(t, "the filters to go", func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.closed })

	// The adapter is free for the next core.
	c2 := h.dial(t)
	hello(t, c2)
	if reply := roundTrip(t, c2, openMessage); reply.Type != tunapi.TypeOpened {
		t.Fatalf("open after the first core left: %+v", reply)
	}
}

func TestASecondCoreIsTurnedAwayWhileATUNRuns(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	roundTrip(t, c, openMessage)
	other := h.dial(t)
	hello(t, other)
	if reply := roundTrip(t, other, openMessage); reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeBusy {
		t.Fatalf("second open: %+v", reply)
	}
	if reply := roundTrip(t, other, tunapi.Message{Type: tunapi.TypeRoute}); reply.Type != tunapi.TypeError {
		t.Fatalf("the second core routed the first one's TUN: %+v", reply)
	}
	// Its close leaves the first core's session alone.
	roundTrip(t, other, tunapi.Message{Type: tunapi.TypeClose})
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute}); reply.Type != tunapi.TypeRouted {
		t.Fatalf("route: %+v", reply)
	}
}

func TestOnlyALoopbackProxyIsAccepted(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	for _, proxy := range []string{"8.8.8.8:1080", "example.com:1080", ""} {
		m := openMessage
		m.Proxy = proxy
		if reply := roundTrip(t, c, m); reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeBadRequest {
			t.Fatalf("proxy %q: %+v", proxy, reply)
		}
	}
}

// A core of a protocol the service does not speak learns why, and can
// still update or remove it.
func TestAnIncompatibleCoreCanStillUpdateTheService(t *testing.T) {
	updated := make(chan string, 1)
	h := newHarness(t, Config{Update: func(path string) error { updated <- path; return nil }})
	c := h.dial(t)
	reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeHello, Protocol: tunapi.Protocol + 7, MinProtocol: tunapi.Protocol + 5, Version: "8.0.0"})
	if reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeProtocol || reply.Protocol != tunapi.Protocol || reply.Version != "1.2.3" {
		t.Fatalf("hello: %+v", reply)
	}
	if reply := roundTrip(t, c, openMessage); reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeProtocol {
		t.Fatalf("open: %+v", reply)
	}
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeUpdate, Path: `C:\new\firefox-vpn-tun.exe`}); reply.Type != tunapi.TypeUpdating {
		t.Fatalf("update: %+v", reply)
	}
	if path := <-updated; path != `C:\new\firefox-vpn-tun.exe` {
		t.Fatalf("updated from %q", path)
	}
	select {
	case <-h.s.Ended():
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not end for the restart")
	}
	if h.s.Reason() != "updated" {
		t.Fatalf("reason %q", h.s.Reason())
	}
}

func TestARefusedUpdateLeavesTheServiceRunning(t *testing.T) {
	h := newHarness(t, Config{Update: func(string) error { return errors.New("not signed") }})
	c := h.dial(t)
	hello(t, c)
	reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeUpdate, Path: `C:\x.exe`})
	if reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeUpdateRefused || !strings.Contains(reply.Message, "not signed") {
		t.Fatalf("update: %+v", reply)
	}
	if reply := roundTrip(t, c, openMessage); reply.Type != tunapi.TypeOpened {
		t.Fatalf("open after a refused update: %+v", reply)
	}
}

func TestUninstallEndsTheSessionAndTheService(t *testing.T) {
	removed := atomic.Bool{}
	h := newHarness(t, Config{Uninstall: func() error { removed.Store(true); return nil }})
	c := h.dial(t)
	hello(t, c)
	roundTrip(t, c, openMessage)
	dev := <-h.devices
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeUninstall}); reply.Type != tunapi.TypeUninstalling {
		t.Fatalf("uninstall: %+v", reply)
	}
	<-h.s.Ended()
	if !removed.Load() || !dev.closed.Load() || h.s.Reason() != "uninstalled" {
		t.Fatalf("removed %v, adapter closed %v, reason %q", removed.Load(), dev.closed.Load(), h.s.Reason())
	}
}

// When the adapter goes away (disabled in the network settings), the core
// is told and the session ends.
// A user who did not install the service (a Unix socket's peer) gets
// nothing but the reason: no session, no update, no uninstall, whatever
// protocol they claim.
func TestAStrangerGetsNothing(t *testing.T) {
	removed := atomic.Bool{}
	h := newHarness(t, Config{Uninstall: func() error { removed.Store(true); return nil }})
	h.stranger.Store(true)
	for _, claim := range []tunapi.Message{tunapi.Hello("9.9.9"), {Type: tunapi.TypeHello, Protocol: 99, MinProtocol: 99}} {
		c := h.dial(t)
		reply := roundTrip(t, c, claim)
		if e, ok := tunapi.AsError(reply).(*tunapi.Error); !ok || e.Code != tunapi.CodeNotAllowed {
			t.Fatalf("hello %+v: %+v", claim, reply)
		}
		// The service hung up.
		if err := c.Send(tunapi.Message{Type: tunapi.TypeUninstall}); err == nil {
			if m, err := c.Receive(); err == nil {
				t.Fatalf("answered %+v after refusing", m)
			}
		}
	}
	if removed.Load() {
		t.Fatal("a stranger removed the service")
	}
}

func TestALostAdapterIsReported(t *testing.T) {
	old := adapterCheck
	adapterCheck = 10 * time.Millisecond
	t.Cleanup(func() { adapterCheck = old })
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	roundTrip(t, c, openMessage)
	roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute})
	dev := <-h.devices
	h.up.Store(false)
	m, err := c.Receive()
	if err != nil || m.Type != tunapi.TypeFailed || m.Code != tunapi.CodeAdapterLost {
		t.Fatalf("event %+v %v", m, err)
	}
	waitFor(t, "the adapter to close", dev.closed.Load)
}

func TestHelloComesFirst(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	if reply := roundTrip(t, c, openMessage); reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeBadRequest {
		t.Fatalf("open before hello: %+v", reply)
	}
}

// socksServer is a SOCKS5 server with a login that echoes what it reads,
// after a line naming the target it was asked for.
func socksServer(t *testing.T, user, password string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				head := make([]byte, 2)
				if _, err := io.ReadFull(r, head); err != nil {
					return
				}
				methods := make([]byte, head[1])
				io.ReadFull(r, methods)
				c.Write([]byte{5, 2})
				ver := make([]byte, 2)
				io.ReadFull(r, ver)
				u := make([]byte, ver[1])
				io.ReadFull(r, u)
				pl := make([]byte, 1)
				io.ReadFull(r, pl)
				p := make([]byte, pl[0])
				io.ReadFull(r, p)
				if string(u) != user || string(p) != password {
					c.Write([]byte{1, 1})
					return
				}
				c.Write([]byte{1, 0})
				req := make([]byte, 4)
				io.ReadFull(r, req)
				var host string
				switch req[3] {
				case 1:
					a := make([]byte, 4)
					io.ReadFull(r, a)
					host = net.IP(a).String()
				case 3:
					l := make([]byte, 1)
					io.ReadFull(r, l)
					n := make([]byte, l[0])
					io.ReadFull(r, n)
					host = string(n)
				}
				pb := make([]byte, 2)
				io.ReadFull(r, pb)
				target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
				if strings.HasPrefix(host, "refuse") {
					c.Write([]byte{5, 2, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				c.Write([]byte(target + "\n"))
				io.Copy(c, r)
			}()
		}
	}()
	return ln.Addr().String()
}

// testSession is a session forwarding to a SOCKS5 server, without an
// adapter.
func testSession(t *testing.T, proxy string) *session {
	t.Helper()
	dial, err := tun.SOCKS5Dialer(proxy, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &session{dns: tun.NewDesktopFakeDNS(), dial: dial, ctx: ctx, cancel: cancel}
}

// flowEnds are the two ends of a forwarded app connection.
type flowEnds struct {
	app      net.Conn
	refused  chan error
	accepted chan struct{}
	accept   func() (net.Conn, error)
	refuse   func(error)
}

func newFlow() *flowEnds {
	app, stack := net.Pipe()
	f := &flowEnds{app: app, refused: make(chan error, 1), accepted: make(chan struct{}, 1)}
	f.accept = func() (net.Conn, error) { f.accepted <- struct{}{}; return stack, nil }
	f.refuse = func(err error) { f.refused <- err; stack.Close() }
	return f
}

func TestAConnectionToAFakeAddressGoesToItsNameThroughTheCore(t *testing.T) {
	s := testSession(t, socksServer(t, "u", "p"))
	q := dnsQuery(t, "example.com.")
	reply, err := s.dns.Answer(q)
	if err != nil {
		t.Fatal(err)
	}
	addr := lastA(t, reply)
	f := newFlow()
	go s.forward(appSource, netip.AddrPortFrom(addr, 443), f.accept, f.refuse)
	select {
	case <-f.accepted:
	case err := <-f.refused:
		t.Fatalf("refused: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("not accepted")
	}
	_ = f.app.SetDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(f.app).ReadString('\n')
	if err != nil || line != "example.com:443\n" {
		t.Fatalf("the core was asked for %q, %v", line, err)
	}
	f.app.Close()
}

func TestFlowsThatLeadNowhereAreRefused(t *testing.T) {
	s := testSession(t, socksServer(t, "u", "p"))
	for _, dst := range []string{
		"10.10.34.35:443",  // the filtering DNS's block page
		"198.18.0.2:853",   // the adapter's own DNS address
		"198.19.200.1:443", // a fake address nobody was given
		"[::ffff:10.10.34.36]:80",
	} {
		f := newFlow()
		go s.forward(appSource, netip.MustParseAddrPort(dst), f.accept, f.refuse)
		select {
		case <-f.refused:
		case <-f.accepted:
			t.Fatalf("%s was accepted", dst)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no answer", dst)
		}
	}
	// The core refusing a destination resets the app's connection.
	reply, _ := s.dns.Answer(dnsQuery(t, "refuse.example."))
	f := newFlow()
	go s.forward(appSource, netip.AddrPortFrom(lastA(t, reply), 443), f.accept, f.refuse)
	select {
	case <-f.refused:
	case <-f.accepted:
		t.Fatal("a destination the core refused was accepted")
	case <-time.After(5 * time.Second):
		t.Fatal("no answer")
	}
}

func TestDNSOverTCPIsAnsweredWithoutTheCore(t *testing.T) {
	s := testSession(t, "127.0.0.1:1") // nothing listens: the core is not asked
	f := newFlow()
	go s.forward(appSource, netip.MustParseAddrPort("8.8.8.8:53"), f.accept, f.refuse)
	q := dnsQuery(t, "example.org.")
	frame := append([]byte{byte(len(q) >> 8), byte(len(q))}, q...)
	_ = f.app.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := f.app.Write(frame); err != nil {
		t.Fatal(err)
	}
	size := make([]byte, 2)
	if _, err := io.ReadFull(f.app, size); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, binary.BigEndian.Uint16(size))
	if _, err := io.ReadFull(f.app, reply); err != nil {
		t.Fatal(err)
	}
	if name, ok := s.dns.Name(lastA(t, reply)); !ok || name != "example.org" {
		t.Fatalf("answered %q", name)
	}
	f.app.Close()
}
