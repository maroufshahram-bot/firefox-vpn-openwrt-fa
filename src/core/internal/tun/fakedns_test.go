package tun

import (
	"io"
	"net"
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func query(t *testing.T, name string, typ dnsmessage.Type) []byte {
	t.Helper()
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{ID: 7, RecursionDesired: true},
		Questions: []dnsmessage.Question{{
			Name: dnsmessage.MustNewName(name), Type: typ, Class: dnsmessage.ClassINET,
		}},
	}
	b, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func answers(t *testing.T, f *FakeDNS, name string, typ dnsmessage.Type) (dnsmessage.Message, []netip.Addr) {
	t.Helper()
	b, err := f.Answer(query(t, name, typ))
	if err != nil {
		t.Fatal(err)
	}
	var m dnsmessage.Message
	if err := m.Unpack(b); err != nil {
		t.Fatal(err)
	}
	var addrs []netip.Addr
	for _, a := range m.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			addrs = append(addrs, netip.AddrFrom4(r.A))
		}
	}
	return m, addrs
}

func TestFakeDNSGivesEachNameAnAddressAndRemembersIt(t *testing.T) {
	f := NewFakeDNS()
	m, a := answers(t, f, "Example.COM.", dnsmessage.TypeA)
	if m.Header.ID != 7 || !m.Header.Response || m.Header.RCode != dnsmessage.RCodeSuccess || len(a) != 1 {
		t.Fatalf("reply %+v %v", m.Header, a)
	}
	if !IsFake(a[0]) {
		t.Fatalf("%s is not in the fake range", a[0])
	}
	if _, again := answers(t, f, "example.com.", dnsmessage.TypeA); again[0] != a[0] {
		t.Fatalf("the same name got %s and %s", a[0], again[0])
	}
	_, other := answers(t, f, "example.org.", dnsmessage.TypeA)
	if other[0] == a[0] {
		t.Fatal("two names share an address")
	}
	target, ok := f.Target(netip.AddrPortFrom(a[0], 443))
	if !ok || target != "example.com:443" {
		t.Fatalf("target %q %v", target, ok)
	}
}

func TestFakeDNSAnswersAAAAWithNothing(t *testing.T) {
	m, a := answers(t, NewFakeDNS(), "example.com.", dnsmessage.TypeAAAA)
	if m.Header.RCode != dnsmessage.RCodeSuccess || len(m.Answers) != 0 || len(a) != 0 {
		t.Fatalf("AAAA: %+v", m)
	}
}

func TestFakeDNSKeepsLocalhost(t *testing.T) {
	_, a := answers(t, NewFakeDNS(), "localhost.", dnsmessage.TypeA)
	if len(a) != 1 || a[0] != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("localhost: %v", a)
	}
}

func TestFakeDNSReusesAddressesAndForgetsOldNames(t *testing.T) {
	f := newFakeDNS(1, 4) // offsets 1 and 2
	_, a := answers(t, f, "a.test.", dnsmessage.TypeA)
	answers(t, f, "b.test.", dnsmessage.TypeA)
	_, c := answers(t, f, "c.test.", dnsmessage.TypeA)
	if c[0] != a[0] {
		t.Fatalf("the range did not wrap: %s, %s", a[0], c[0])
	}
	if name, _ := f.Name(a[0]); name != "c.test" {
		t.Fatalf("the reused address stands for %q", name)
	}
	if _, ok := f.Target(netip.AddrPortFrom(netip.MustParseAddr("198.19.255.200"), 80)); ok {
		t.Fatal("an unknown fake address must not be dialed")
	}
}

// The desktop's FakeDNS keeps the adapter's own addresses out, also when
// the range wraps, so a connection to them is refused like a forgotten
// fake address.
func TestDesktopFakeDNSKeepsTheAdapterAddressesOut(t *testing.T) {
	f := NewDesktopFakeDNS()
	_, a := answers(t, f, "first.test.", dnsmessage.TypeA)
	if a[0] != netip.MustParseAddr("198.18.0.4") {
		t.Fatalf("the first address is %s, want 198.18.0.4", a[0])
	}
	small := newFakeDNS(4, 7) // offsets 4 and 5
	small.fixed = f.fixed
	for _, name := range []string{"a.test.", "b.test.", "c.test."} {
		_, got := answers(t, small, name, dnsmessage.TypeA)
		if DesktopNet.Contains(got[0]) {
			t.Fatalf("%s got the adapter's address %s", name, got[0])
		}
	}
	for _, addr := range []string{"198.18.0.1", "198.18.0.2"} {
		if _, ok := f.Target(netip.AddrPortFrom(netip.MustParseAddr(addr), 853)); ok {
			t.Fatalf("a connection to the adapter's %s would be sent on", addr)
		}
	}
}

// Windows' network check gets Microsoft's real answer, in both families,
// and keeps no fake address.
func TestDesktopFakeDNSAnswersTheNetworkCheckForReal(t *testing.T) {
	f := NewDesktopFakeDNS()
	_, a := answers(t, f, "dns.msftncsi.com.", dnsmessage.TypeA)
	if len(a) != 1 || a[0] != netip.MustParseAddr("131.107.255.255") {
		t.Fatalf("A: %v", a)
	}
	b, err := f.Answer(query(t, "dns.msftncsi.com.", dnsmessage.TypeAAAA))
	if err != nil {
		t.Fatal(err)
	}
	var m dnsmessage.Message
	if err := m.Unpack(b); err != nil || len(m.Answers) != 1 {
		t.Fatalf("AAAA: %+v %v", m, err)
	}
	if r, ok := m.Answers[0].Body.(*dnsmessage.AAAAResource); !ok || netip.AddrFrom16(r.AAAA) != netip.MustParseAddr("fd3e:4f5a:5b81::1") {
		t.Fatalf("AAAA: %+v", m.Answers[0])
	}
	if _, a := answers(t, NewFakeDNS(), "dns.msftncsi.com.", dnsmessage.TypeA); !IsFake(a[0]) {
		t.Fatal("the phone's FakeDNS has no fixed names")
	}
}

func TestTargetKeepsRealAddresses(t *testing.T) {
	f := NewFakeDNS()
	for in, want := range map[string]string{
		"1.2.3.4:443":                 "1.2.3.4:443",
		"[2001:db8::1]:80":            "[2001:db8::1]:80",
		"[::ffff:149.154.167.51]:443": "149.154.167.51:443",
	} {
		got, ok := f.Target(netip.MustParseAddrPort(in))
		if !ok || got != want {
			t.Fatalf("%s: %q %v", in, got, ok)
		}
	}
}

func TestFakeDNSServesDNSOverTCP(t *testing.T) {
	f := NewFakeDNS()
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- f.ServeStream(server); server.Close() }()
	for _, name := range []string{"one.test.", "two.test."} {
		q := query(t, name, dnsmessage.TypeA)
		frame := append([]byte{byte(len(q) >> 8), byte(len(q))}, q...)
		if _, err := client.Write(frame); err != nil {
			t.Fatal(err)
		}
		var size [2]byte
		if _, err := io.ReadFull(client, size[:]); err != nil {
			t.Fatal(err)
		}
		reply := make([]byte, int(size[0])<<8|int(size[1]))
		if _, err := io.ReadFull(client, reply); err != nil {
			t.Fatal(err)
		}
		var m dnsmessage.Message
		if err := m.Unpack(reply); err != nil || len(m.Answers) != 1 {
			t.Fatalf("%s: %+v %v", name, m, err)
		}
	}
	client.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestVPNAddresses(t *testing.T) {
	if !IsVPNAddress(netip.MustParseAddr("172.19.0.2")) || IsVPNAddress(netip.MustParseAddr("172.19.0.9")) {
		t.Fatal("the VPN's own range is 172.19.0.0/30")
	}
}

func TestFilterAddresses(t *testing.T) {
	for _, a := range []string{"10.10.34.36", "10.10.34.34", "2001:4188:2:600:10:10:34:36", "::ffff:10.10.34.35"} {
		if !IsFilterAddress(netip.MustParseAddr(a)) {
			t.Errorf("%s is a filtering answer", a)
		}
	}
	for _, a := range []string{"10.10.35.1", "8.8.8.8", "2001:4188:2:600::1"} {
		if IsFilterAddress(netip.MustParseAddr(a)) {
			t.Errorf("%s is not", a)
		}
	}
}
