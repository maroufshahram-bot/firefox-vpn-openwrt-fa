//go:build with_gvisor

package tunstack

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sagernet/gvisor/pkg/buffer"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/icmp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
	tun "github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"golang.org/x/net/dns/dnsmessage"

	faketun "github.com/armanlife/firefox-vpn-client/core/internal/tun"
)

// The tests put a second TCP/IP stack, standing in for the computer and its
// apps, at the other end of the device: what the device's stack sends goes
// to the computer's, and the other way round, so the handler sees real
// flows without an adapter or administrator rights.

var (
	hostV4 = netip.MustParsePrefix("198.18.0.1/30")
	hostV6 = netip.MustParsePrefix("fdfe:dcba:9876::1/126")
)

// fakeTun is the device: the stack's link endpoint, and the computer's.
type fakeTun struct {
	server, client *channel.Endpoint
	toClient       chan []byte
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup

	mu       sync.Mutex
	received [][]byte // what the computer received
}

func copyPacket(pkt *stack.PacketBuffer) []byte {
	v := pkt.ToView()
	defer v.Release()
	return append([]byte(nil), v.AsSlice()...)
}

func inject(ep *channel.Endpoint, b []byte) {
	proto := header.IPv4ProtocolNumber
	if header.IPVersion(b) == header.IPv6Version {
		proto = header.IPv6ProtocolNumber
	}
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(b)})
	ep.InjectInbound(proto, pkt)
	pkt.DecRef()
}

func newFakeTun() *fakeTun {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeTun{
		server:   channel.New(1024, 1500, ""),
		client:   channel.New(1024, 1500, ""),
		toClient: make(chan []byte, 1024),
		ctx:      ctx,
		cancel:   cancel,
	}
	pump := func(from *channel.Endpoint, deliver func([]byte)) {
		defer f.wg.Done()
		for {
			pkt := from.ReadContext(ctx)
			if pkt == nil {
				return
			}
			b := copyPacket(pkt)
			pkt.DecRef()
			deliver(b)
		}
	}
	f.wg.Add(3)
	go pump(f.server, func(b []byte) { f.toClient <- b })
	go pump(f.client, func(b []byte) { inject(f.server, b) })
	go func() {
		defer f.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case b := <-f.toClient:
				f.mu.Lock()
				f.received = append(f.received, b)
				f.mu.Unlock()
				inject(f.client, b)
			}
		}
	}()
	return f
}

func (f *fakeTun) Read([]byte) (int, error)             { return 0, io.EOF }
func (f *fakeTun) Write(p []byte) (int, error)          { return len(p), nil }
func (f *fakeTun) Name() (string, error)                { return "fake", nil }
func (f *fakeTun) Start() error                         { return nil }
func (f *fakeTun) UpdateRouteOptions(tun.Options) error { return nil }
func (f *fakeTun) NewEndpoint() (stack.LinkEndpoint, stack.NICOptions, error) {
	return f.server, stack.NICOptions{}, nil
}

// WritePacket is the device's own answers (DNS, refusals) to the computer.
func (f *fakeTun) WritePacket(pkt *stack.PacketBuffer) (int, error) {
	b := copyPacket(pkt)
	select {
	case f.toClient <- b:
	case <-f.ctx.Done():
	}
	return len(b), nil
}

func (f *fakeTun) Close() error {
	f.cancel()
	f.wg.Wait()
	return nil
}

// sawUnreachable reports whether the computer received an ICMP port
// unreachable.
func (f *fakeTun) sawUnreachable() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.received {
		if header.IPVersion(b) != header.IPv4Version {
			continue
		}
		ip := header.IPv4(b)
		if ip.TransportProtocol() != header.ICMPv4ProtocolNumber {
			continue
		}
		m := header.ICMPv4(ip.Payload())
		if m.Type() == header.ICMPv4DstUnreachable && m.Code() == header.ICMPv4PortUnreachable {
			return true
		}
	}
	return false
}

// computer is the stack of the apps, with the adapter's addresses.
func computer(t *testing.T, ep *channel.Endpoint) *stack.Stack {
	t.Helper()
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol, icmp.NewProtocol4, icmp.NewProtocol6},
	})
	if err := s.CreateNIC(1, ep); err != nil {
		t.Fatal(err)
	}
	for _, p := range []netip.Prefix{hostV4, hostV6} {
		proto := ipv4.ProtocolNumber
		if p.Addr().Is6() {
			proto = ipv6.ProtocolNumber
		}
		addr := tcpip.ProtocolAddress{Protocol: proto, AddressWithPrefix: tcpip.AddrFromSlice(p.Addr().AsSlice()).WithPrefix()}
		if err := s.AddProtocolAddress(1, addr, stack.AddressProperties{}); err != nil {
			t.Fatal(err)
		}
	}
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: 1},
		{Destination: header.IPv6EmptySubnet, NIC: 1},
	})
	t.Cleanup(s.Close)
	return s
}

// startDevice runs a device with h at the end of a fake adapter.
func startDevice(t *testing.T, h Handler) (*fakeTun, *stack.Stack) {
	t.Helper()
	f, s, _ := startDeviceOf(t, h)
	return f, s
}

// startDeviceOf is startDevice, with the device.
func startDeviceOf(t *testing.T, h Handler) (*fakeTun, *stack.Stack, *Device) {
	t.Helper()
	f := newFakeTun()
	ctx, cancel := context.WithCancel(context.Background())
	d := &Device{
		cfg: Config{Name: "test", Handler: h},
		options: tun.Options{
			Name:         "test",
			Inet4Address: []netip.Prefix{hostV4},
			Inet6Address: []netip.Prefix{hostV6},
			MTU:          1500,
		},
		tun:    f,
		finder: control.NewDefaultInterfaceFinder(),
		ctx:    ctx,
		cancel: cancel,
	}
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return f, computer(t, f.client), d
}

func fullAddr(ap netip.AddrPort) tcpip.FullAddress {
	return tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(ap.Addr().AsSlice()), Port: ap.Port()}
}

func protoOf(ap netip.AddrPort) tcpip.NetworkProtocolNumber {
	if ap.Addr().Is6() {
		return ipv6.ProtocolNumber
	}
	return ipv4.ProtocolNumber
}

// echoHandler answers DNS with dns, and accepts TCP to port 443 (replying
// "pong" to "ping") and refuses the rest; flows reports each flow seen.
func echoHandler(dns *faketun.FakeDNS, flows chan<- *Flow) Handler {
	return Handler{
		DNS: dns.Answer,
		TCP: func(f *Flow) {
			flows <- f
			if f.Destination.Port() != 443 {
				f.Refuse(errors.New("refused by the test"))
				return
			}
			conn, err := f.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			buf := make([]byte, 4)
			if _, err := io.ReadFull(conn, buf); err == nil && string(buf) == "ping" {
				_, _ = conn.Write([]byte("pong"))
			}
		},
	}
}

func TestTCPGoesToTheHandlerAndBack(t *testing.T) {
	flows := make(chan *Flow, 4)
	_, s := startDevice(t, echoHandler(faketun.NewDesktopFakeDNS(), flows))
	for _, dst := range []netip.AddrPort{
		netip.MustParseAddrPort("1.2.3.4:443"),
		netip.MustParseAddrPort("[2001:db8::1]:443"),
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := gonet.DialContextTCP(ctx, s, fullAddr(dst), protoOf(dst))
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", dst, err)
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write([]byte("ping")); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, 4)
		if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != "pong" {
			t.Fatalf("%s: reply %q %v", dst, reply, err)
		}
		conn.Close()
		f := <-flows
		if f.Destination != dst {
			t.Fatalf("the handler saw %s, want %s", f.Destination, dst)
		}
		if want := hostV4.Addr(); dst.Addr().Is4() && f.Source.Addr() != want {
			t.Fatalf("the flow comes from %s, want %s", f.Source, want)
		}
	}
}

// A refused flow resets the app's connection at once.
func TestARefusedFlowResetsTheApp(t *testing.T) {
	flows := make(chan *Flow, 4)
	_, s := startDevice(t, echoHandler(faketun.NewDesktopFakeDNS(), flows))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := gonet.DialContextTCP(ctx, s, fullAddr(netip.MustParseAddrPort("1.2.3.4:444")), ipv4.ProtocolNumber)
	if err == nil {
		conn.Close()
		t.Fatal("the refused connection was established")
	}
	if ctx.Err() != nil || time.Since(start) > 3*time.Second {
		t.Fatalf("the refusal took %s: %v", time.Since(start), err)
	}
}

// DNS to any server is answered by the handler, from that server's
// address.
func TestDNSToAnyServerIsAnswered(t *testing.T) {
	dns := faketun.NewDesktopFakeDNS()
	_, s := startDevice(t, echoHandler(dns, make(chan *Flow, 4)))
	server := netip.MustParseAddrPort("8.8.8.8:53")
	conn, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(server.Addr().AsSlice()), Port: 53}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 77, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName("example.com."), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	query, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(query); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	var reply dnsmessage.Message
	if err := reply.Unpack(buf[:n]); err != nil || reply.Header.ID != 77 || len(reply.Answers) != 1 {
		t.Fatalf("reply %+v %v", reply, err)
	}
	a := reply.Answers[0].Body.(*dnsmessage.AResource)
	addr := netip.AddrFrom4(a.A)
	if name, ok := dns.Name(addr); !ok || name != "example.com" {
		t.Fatalf("the answer %s stands for %q", addr, name)
	}
}

// UDP that TakeUDP takes goes to UDP, with each packet's destination, and
// the replies reach the app from the address it chose; what TakeUDP
// leaves is refused as before.
func TestTakenUDPGoesToTheHandlerAndBack(t *testing.T) {
	h := echoHandler(faketun.NewDesktopFakeDNS(), make(chan *Flow, 4))
	sources := make(chan netip.AddrPort, 4)
	h.TakeUDP = func(source, dst netip.AddrPort) bool {
		sources <- source
		return dst.Port() == 3478
	}
	h.UDP = func(f *PacketFlow) {
		f.Accept()
		b := make([]byte, 1500)
		for {
			n, dst, err := f.Read(b)
			if err != nil {
				return
			}
			if err := f.Write(append([]byte("echo:"), b[:n]...), dst); err != nil {
				return
			}
		}
	}
	f, s := startDevice(t, h)
	dst := netip.MustParseAddrPort("1.2.3.4:3478")
	conn, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(dst.Addr().AsSlice()), Port: dst.Port()}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i, msg := range []string{"one", "two"} {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 1500)
		n, err := conn.Read(b)
		if err != nil || string(b[:n]) != "echo:"+msg {
			t.Fatalf("reply %q %v", b[:n], err)
		}
		if i == 0 {
			// The first packet is judged (by sing-tun's dispatcher and its UDP
			// forwarder); a flow's later ones are not.
			if src := <-sources; src.Addr() != hostV4.Addr() {
				t.Fatalf("the flow comes from %s, want the adapter's address", src)
			}
			for len(sources) > 0 {
				<-sources
			}
		}
	}
	if len(sources) != 0 {
		t.Fatalf("the flow's later packets were judged again: %d", len(sources))
	}

	quic, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{1, 2, 3, 4}), Port: 443}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer quic.Close()
	if _, err := quic.Write([]byte("quic initial")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !f.sawUnreachable() {
		if time.Now().After(deadline) {
			t.Fatal("UDP that TakeUDP left was not refused")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A refused port stays refused while it sends; after ResetFlows its next
// packet is judged again (a program was added to the ones that skip the
// VPN).
func TestResetFlowsJudgesARefusedPortAgain(t *testing.T) {
	var take atomic.Bool
	h := echoHandler(faketun.NewDesktopFakeDNS(), make(chan *Flow, 4))
	h.TakeUDP = func(_, _ netip.AddrPort) bool { return take.Load() }
	h.UDP = func(f *PacketFlow) {
		f.Accept()
		b := make([]byte, 1500)
		for {
			n, dst, err := f.Read(b)
			if err != nil || f.Write(b[:n], dst) != nil {
				return
			}
		}
	}
	f, s, d := startDeviceOf(t, h)
	conn, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{1, 2, 3, 4}), Port: 3478}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	answered := func() bool {
		// The ICMP unreachable comes back as an error, once.
		if _, err := conn.Write([]byte("call")); err != nil {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		b := make([]byte, 64)
		n, err := conn.Read(b)
		return err == nil && string(b[:n]) == "call"
	}
	if answered() {
		t.Fatal("a refused port was answered")
	}
	deadline := time.Now().Add(3 * time.Second)
	for !f.sawUnreachable() {
		if time.Now().After(deadline) {
			t.Fatal("no ICMP port unreachable")
		}
		time.Sleep(10 * time.Millisecond)
	}
	take.Store(true)
	if answered() {
		t.Fatal("a refused port was judged again without ResetFlows")
	}
	d.ResetFlows()
	for !answered() {
		if time.Now().After(deadline.Add(3 * time.Second)) {
			t.Fatal("after ResetFlows the port is still refused")
		}
	}
}

// Other UDP is refused with an ICMP port unreachable, so apps fall back to
// TCP at once (QUIC to HTTPS over TCP).
func TestOtherUDPIsRefused(t *testing.T) {
	f, s := startDevice(t, echoHandler(faketun.NewDesktopFakeDNS(), make(chan *Flow, 4)))
	conn, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{1, 2, 3, 4}), Port: 443}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("quic initial")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !f.sawUnreachable() {
		if time.Now().After(deadline) {
			t.Fatal("no ICMP port unreachable for UDP to port 443")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
