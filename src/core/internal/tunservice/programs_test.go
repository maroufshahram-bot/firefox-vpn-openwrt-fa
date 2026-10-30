package tunservice

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/tun"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// appSource is an app's connection on the adapter, as the stack sees it.
var appSource = netip.MustParseAddrPort("198.18.0.1:50000")

// fakeOwners makes findProgram answer by the source port; it counts the
// lookups.
func fakeOwners(t *testing.T, byPort map[uint16]string) *atomic.Int32 {
	t.Helper()
	old := findProgram
	var lookups atomic.Int32
	findProgram = func(_ bool, source netip.AddrPort) (string, error) {
		lookups.Add(1)
		if path, ok := byPort[source.Port()]; ok {
			return path, nil
		}
		return "", errors.New("no such socket")
	}
	t.Cleanup(func() { findProgram = old })
	return &lookups
}

// port is appSource on another port.
func port(p uint16) netip.AddrPort { return netip.AddrPortFrom(appSource.Addr(), p) }

// coreSession is a session whose core speaks protocol 2 and was told the
// TUN is routed; core is its end of the connection.
func coreSession(t *testing.T, programs ...tunapi.Program) (s *session, core *tunapi.Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s = &session{owner: tunapi.NewConn(b), protocol: 2, dns: tun.NewDesktopFakeDNS(), ctx: ctx, cancel: cancel}
	s.live.Store(true)
	s.setPrograms(programs)
	return s, tunapi.NewConn(a)
}

// told reads what the core is told within a moment: "" for nothing.
func told(t *testing.T, core *tunapi.Conn) string {
	t.Helper()
	_ = core.Raw().SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	defer core.Raw().SetReadDeadline(time.Time{})
	m, err := core.Receive()
	if err != nil {
		return ""
	}
	if m.Type != tunapi.TypeUDPRefused {
		t.Fatalf("the core was sent %+v", m)
	}
	return m.Program
}

// systemPath is a program of the system itself on this OS.
func systemPath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\svchost.exe`
	}
	return "/usr/sbin/chronyd"
}

// Programs are known by their file name without case, or by a pattern,
// which a name wins over.
func TestProgramsAreKnownByTheirFileName(t *testing.T) {
	set := newProgramSet([]tunapi.Program{
		{Name: " Steam.EXE "},
		{Name: "Telegram.exe", UDPOnly: true},
		{Name: ""},
		{Name: "steam*", UDPOnly: true},
		{Name: "*game*"},
	})
	if set.size() != 4 {
		t.Fatalf("set %+v", set)
	}
	for _, c := range []struct {
		name        string
		ok, udpOnly bool
	}{
		{"steam.exe", true, false},
		{"TELEGRAM.EXE", true, true},
		{"steamwebhelper.exe", true, true},
		{"MyGameClient.exe", true, false},
		{"curl.exe", false, false},
		{"", false, false},
	} {
		if udpOnly, ok := set.lookup(c.name); ok != c.ok || udpOnly != c.udpOnly {
			t.Errorf("lookup(%q) = %v %v, want %v %v", c.name, udpOnly, ok, c.udpOnly, c.ok)
		}
	}
	for path, want := range map[string]string{
		`C:\Program Files (x86)\Steam\steam.exe`: "steam.exe",
		"/usr/bin/curl":                          "curl",
		"curl":                                   "curl",
	} {
		if got := programName(path); got != want {
			t.Errorf("programName(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCallsAndGamesAreToldFromQUICAndLocalUDP(t *testing.T) {
	for dst, want := range map[string]bool{
		"1.2.3.4:3478":               true, // STUN
		"74.125.250.129:19302":       true,
		"198.18.0.9:50000":           true, // a fake address: a name
		"[2001:4860:4860::1]:3478":   true,
		"1.2.3.4:443":                false, // QUIC
		"8.8.8.8:853":                false,
		"129.6.15.28:123":            false, // the clock
		"192.168.1.5:3478":           false,
		"10.0.0.7:50000":             false,
		"239.255.255.250:1900":       false, // discovery
		"255.255.255.255:67":         false,
		"[fe80::1]:5353":             false,
		"[ff02::fb]:5353":            false,
		"[::ffff:192.168.1.9]:50000": false,
	} {
		if got := callLike(netip.MustParseAddrPort(dst)); got != want {
			t.Errorf("callLike(%s) = %v, want %v", dst, got, want)
		}
	}
}

// A program whose traffic skips the VPN gets all its UDP taken; one whose
// UDP alone skips it keeps QUIC in the VPN; the others are refused, and a
// call of theirs is told to the core once, not one of the system's.
func TestUDPOfTheProgramsThatSkipTheVPNIsTaken(t *testing.T) {
	lookups := fakeOwners(t, map[uint16]string{
		1000: `C:\Games\Game.exe`,
		1001: "/opt/telegram/Telegram.exe",
		1002: systemPath(),
		1003: `C:\Users\u\AppData\Local\Discord\app-1.0.9\Discord.exe`,
	})
	s, core := coreSession(t, tunapi.Program{Name: "game.exe"}, tunapi.Program{Name: "telegram.exe", UDPOnly: true})
	call, quic := netip.MustParseAddrPort("1.2.3.4:3478"), netip.MustParseAddrPort("1.2.3.4:443")
	for _, c := range []struct {
		port uint16
		dst  netip.AddrPort
		want bool
	}{
		{1000, call, true},
		{1000, quic, true},
		{1001, call, true},
		{1001, quic, false},
		{1002, call, false},
		{1003, quic, false},
		{1004, call, false}, // no program found
	} {
		if got := s.takeUDP(port(c.port), c.dst); got != c.want {
			t.Errorf("takeUDP(%d, %s) = %v, want %v", c.port, c.dst, got, c.want)
		}
	}
	if got := told(t, core); got != "" {
		t.Fatalf("the core was told of %q: neither the system's UDP nor QUIC is a call", got)
	}
	if s.takeUDP(port(1003), call) {
		t.Fatal("a call of a program that does not skip the VPN was taken")
	}
	if got := told(t, core); got != "Discord.exe" {
		t.Fatalf("the core was told of %q, want Discord.exe", got)
	}
	s.takeUDP(port(1003), netip.MustParseAddrPort("5.6.7.8:50000"))
	if got := told(t, core); got != "" {
		t.Fatalf("told again, of %q", got)
	}

	// A port's program is looked up once a moment, though its refused
	// packets keep coming.
	before := lookups.Load()
	for range 20 {
		s.takeUDP(port(1003), call)
	}
	if n := lookups.Load() - before; n != 0 {
		t.Fatalf("%d lookups for a port already known", n)
	}

	// Without programs, QUIC is refused without a lookup at all.
	s2, _ := coreSession(t)
	before = lookups.Load()
	if s2.takeUDP(port(1000), quic) || lookups.Load() != before {
		t.Fatal("QUIC without programs was looked up or taken")
	}
}

// The core is told nothing before it reads (the TUN routed) or when it
// speaks protocol 1.
func TestRefusedCallsWaitForACoreThatReads(t *testing.T) {
	fakeOwners(t, map[uint16]string{1003: `C:\Apps\Discord.exe`})
	call := netip.MustParseAddrPort("1.2.3.4:3478")
	s, core := coreSession(t)
	s.live.Store(false)
	s.takeUDP(port(1003), call)
	if got := told(t, core); got != "" {
		t.Fatalf("told %q before the TUN was routed", got)
	}
	s.live.Store(true)
	s.takeUDP(port(1003), call)
	if got := told(t, core); got != "Discord.exe" {
		t.Fatalf("told %q once routed, want Discord.exe", got)
	}
	old, oldCore := coreSession(t)
	old.protocol = 1
	old.takeUDP(port(1003), call)
	if got := told(t, oldCore); got != "" {
		t.Fatalf("a core of protocol 1 was told %q", got)
	}
}

// The TCP of a program whose traffic skips the VPN connects directly;
// that of one whose UDP alone skips it, and of the rest, goes to the core.
func TestTCPOfAProgramThatSkipsTheVPNConnectsDirectly(t *testing.T) {
	fakeOwners(t, map[uint16]string{2000: `C:\Tools\curl.exe`, 2001: `C:\Apps\Telegram.exe`})
	var mu sync.Mutex
	var direct []string
	old := dialDirect
	dialDirect = func(_ context.Context, target string) (net.Conn, error) {
		mu.Lock()
		direct = append(direct, target)
		mu.Unlock()
		a, b := net.Pipe()
		go func() { _, _ = b.Write([]byte("direct\n")); b.Close() }()
		return a, nil
	}
	t.Cleanup(func() { dialDirect = old })
	s := testSession(t, socksServer(t, "u", "p"))
	s.setPrograms([]tunapi.Program{{Name: "CURL.EXE"}, {Name: "telegram.exe", UDPOnly: true}})
	reply, _ := s.dns.Answer(dnsQuery(t, "example.com."))
	dst := netip.AddrPortFrom(lastA(t, reply), 443)
	for _, c := range []struct {
		port uint16
		want string
	}{
		{2000, "direct\n"},
		{2001, "example.com:443\n"}, // the core's answer: the target
		{2002, "example.com:443\n"},
	} {
		f := newFlow()
		go s.forward(port(c.port), dst, f.accept, f.refuse)
		_ = f.app.SetDeadline(time.Now().Add(5 * time.Second))
		line, err := bufio.NewReader(f.app).ReadString('\n')
		if err != nil || line != c.want {
			t.Errorf("port %d got %q %v, want %q", c.port, line, err, c.want)
		}
		f.app.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(direct, []string{"example.com:443"}) {
		t.Fatalf("connected directly to %v, want example.com:443 by name", direct)
	}
}

// fakePacketFlow is an app's UDP port: what the test sends comes out of
// Read, what the relay writes goes to out.
type fakePacketFlow struct {
	in       chan packet
	out      chan packet
	accepted atomic.Bool
	done     chan struct{}
}

type packet struct {
	data string
	addr netip.AddrPort
}

func newFakePacketFlow() *fakePacketFlow {
	return &fakePacketFlow{in: make(chan packet, 8), out: make(chan packet, 8), done: make(chan struct{})}
}

func (f *fakePacketFlow) Accept() { f.accepted.Store(true) }
func (f *fakePacketFlow) Read(b []byte) (int, netip.AddrPort, error) {
	select {
	case p := <-f.in:
		return copy(b, p.data), p.addr, nil
	case <-f.done:
		return 0, netip.AddrPort{}, net.ErrClosed
	}
}
func (f *fakePacketFlow) Write(b []byte, from netip.AddrPort) error {
	f.out <- packet{string(b), from}
	return nil
}

// echoUDP answers each packet with "echo:" and the packet.
func echoUDP(t *testing.T) netip.AddrPort {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		b := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFromUDPAddrPort(b)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDPAddrPort(append([]byte("echo:"), b[:n]...), from)
		}
	}()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort()
}

// UDP goes to its destination directly, a fake address to its name's real
// one, and the replies come from the address the app sent to; a forgotten
// fake address leads nowhere.
func TestDirectUDPGoesOutAndComesBack(t *testing.T) {
	echo := echoUDP(t)
	old := lookupDirect
	lookupDirect = func(_ context.Context, name string) ([]netip.Addr, error) {
		if name == "echo.example" {
			return []netip.Addr{netip.MustParseAddr("::1"), echo.Addr()}, nil
		}
		return nil, errors.New("no such name")
	}
	t.Cleanup(func() { lookupDirect = old })
	s := testSession(t, "127.0.0.1:1")
	reply, _ := s.dns.Answer(dnsQuery(t, "echo.example."))
	fake := netip.AddrPortFrom(lastA(t, reply), echo.Port())
	f := newFakePacketFlow()
	ended := make(chan struct{})
	go func() { s.relay(f); close(ended) }()
	for _, c := range []struct {
		send packet
		want packet
	}{
		{packet{"by name", fake}, packet{"echo:by name", fake}},
		{packet{"by address", echo}, packet{"echo:by address", echo}},
	} {
		f.in <- c.send
		select {
		case got := <-f.out:
			if got != c.want {
				t.Errorf("sent %+v, got %+v, want %+v", c.send, got, c.want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no reply to %+v", c.send)
		}
	}
	if !f.accepted.Load() {
		t.Error("the flow was not accepted")
	}
	f.in <- packet{"nowhere", netip.MustParseAddrPort("198.19.200.1:9")}
	select {
	case got := <-f.out:
		t.Fatalf("a packet to a forgotten fake address was answered: %+v", got)
	case <-time.After(200 * time.Millisecond):
	}
	close(f.done)
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the relay did not end with its flow")
	}
}

// The programs change while the TUN runs, for its session only, and every
// UDP flow is judged again.
func TestTheProgramsChangeWhileTheTUNRuns(t *testing.T) {
	h := newHarness(t, Config{})
	c := h.dial(t)
	hello(t, c)
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypePrograms, Programs: []tunapi.Program{{Name: "a.exe"}}}); reply.Type != tunapi.TypeError || reply.Code != tunapi.CodeBadRequest {
		t.Fatalf("programs before open: %+v", reply)
	}
	open := openMessage
	open.Programs = []tunapi.Program{{Name: "steam.exe"}}
	if reply := roundTrip(t, c, open); reply.Type != tunapi.TypeOpened {
		t.Fatalf("open: %+v", reply)
	}
	dev := <-h.devices
	h.s.mu.Lock()
	sess := h.s.session
	h.s.mu.Unlock()
	if set := sess.programSet(); set.size() != 1 {
		t.Fatalf("opened with %+v", set)
	} else if udpOnly, ok := set.lookup("steam.exe"); !ok || udpOnly {
		t.Fatalf("opened with %+v", set)
	}
	if sess.live.Load() {
		t.Fatal("live before the route")
	}
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypeRoute}); reply.Type != tunapi.TypeRouted {
		t.Fatalf("route: %+v", reply)
	}
	waitFor(t, "the session to be live", sess.live.Load)
	if reply := roundTrip(t, c, tunapi.Message{Type: tunapi.TypePrograms, Programs: []tunapi.Program{{Name: "Discord.exe", UDPOnly: true}}}); reply.Type != tunapi.TypeProgramsSet {
		t.Fatalf("programs: %+v", reply)
	}
	if set := sess.programSet(); set.size() != 1 || !set.names["discord.exe"] {
		t.Fatalf("changed to %v", set)
	}
	if n := dev.resets.Load(); n != 1 {
		t.Fatalf("the flows were reset %d times, want once", n)
	}
	// Another client has no session to change.
	other := h.dial(t)
	hello(t, other)
	if reply := roundTrip(t, other, tunapi.Message{Type: tunapi.TypePrograms}); reply.Type != tunapi.TypeError {
		t.Fatalf("another client changed the programs: %+v", reply)
	}
	if !strings.Contains(sess.trusted[1], "firefox-vpn-core") {
		t.Fatalf("trusted %v", sess.trusted)
	}
	roundTrip(t, c, tunapi.Message{Type: tunapi.TypeClose})
}
