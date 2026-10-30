// Package tun carries a phone's traffic through the proxy: Android's
// VpnService hands the core a TUN device, a userspace TCP/IP stack (gVisor)
// takes its packets apart, and every TCP connection goes out through the
// core's own SOCKS5 proxy, so routing, accounts, and Mozilla's tunnel apply
// as for any proxy client. DNS is answered here with fake addresses that
// stand for names (FakeDNS), so the proxy gets the name, not an address.
// Other UDP is refused: Mozilla's proxy carries TCP only.
package tun

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/dns/dnsmessage"
)

// fakeRange holds the addresses FakeDNS hands out: 198.18.0.0/15, set
// aside for benchmarking (RFC 2544), so no real site uses it.
var fakeRange = netip.MustParsePrefix("198.18.0.0/15")

// fakeTTL is short: an address is reused once the range wraps.
const fakeTTL = 60

// FakeDNS answers A queries with an address from fakeRange per name and
// remembers which name each address stands for. AAAA and other types get
// no answers, so apps use IPv4 and the fake addresses.
type FakeDNS struct {
	mu     sync.Mutex
	byName map[string]netip.Addr
	byAddr map[netip.Addr]string
	// next is the offset of the next address to hand out in fakeRange;
	// offsets run from first to size-2, then start again at first.
	next, first, size uint32
	// fixed names get these real addresses instead (A and AAAA).
	fixed map[string][]netip.Addr
}

// NewFakeDNS is the phone's: the whole range (its VPN interface's own
// addresses are elsewhere, vpnNet).
func NewFakeDNS() *FakeDNS {
	return newFakeDNS(1, 1<<(32-fakeRange.Bits())) // not the network address
}

// DesktopNet holds the desktop TUN adapter's own addresses: the first four
// of fakeRange, which no app on a computer can be using either (WSL and
// Docker take their networks from 172.16.0.0/12, where the phone's are).
// NewDesktopFakeDNS never hands them out, so a connection to one is
// refused as a forgotten fake address.
var DesktopNet = netip.MustParsePrefix("198.18.0.0/30")

// NewDesktopFakeDNS is the desktop TUN mode's: the range after DesktopNet,
// and the real answer for the name of Windows' network check (NCSI), which
// compares it with Microsoft's address and would otherwise report no
// internet access.
func NewDesktopFakeDNS() *FakeDNS {
	f := newFakeDNS(4, 1<<(32-fakeRange.Bits()))
	f.fixed = map[string][]netip.Addr{
		"dns.msftncsi.com": {netip.MustParseAddr("131.107.255.255"), netip.MustParseAddr("fd3e:4f5a:5b81::1")},
	}
	return f
}

// newFakeDNS hands out offsets first to size-2 before reusing them; tests
// use a small size.
func newFakeDNS(first, size uint32) *FakeDNS {
	return &FakeDNS{
		byName: map[string]netip.Addr{},
		byAddr: map[netip.Addr]string{},
		next:   first,
		first:  first,
		size:   size,
	}
}

// IsFake reports whether addr is in the range FakeDNS hands out.
func IsFake(addr netip.Addr) bool { return fakeRange.Contains(addr.Unmap()) }

// Name returns the name a fake address stands for.
func (f *FakeDNS) Name(addr netip.Addr) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name, ok := f.byAddr[addr.Unmap()]
	return name, ok
}

// addrFor returns name's fake address, handing out the next one (and
// forgetting the name that had it) for a new name.
func (f *FakeDNS) addrFor(name string) netip.Addr {
	f.mu.Lock()
	defer f.mu.Unlock()
	if addr, ok := f.byName[name]; ok {
		return addr
	}
	base := fakeRange.Addr().As4()
	n := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
	n += f.next
	addr := netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	f.next++
	if f.next >= f.size-1 { // not the broadcast address
		f.next = f.first
	}
	if old, ok := f.byAddr[addr]; ok {
		delete(f.byName, old)
	}
	f.byAddr[addr] = name
	f.byName[name] = addr
	return addr
}

// Answer builds the reply to a DNS query in wire format.
func (f *FakeDNS) Answer(query []byte) ([]byte, error) {
	var p dnsmessage.Parser
	header, err := p.Start(query)
	if err != nil {
		return nil, err
	}
	if header.Response {
		return nil, errors.New("not a query")
	}
	questions, err := p.AllQuestions()
	if err != nil {
		return nil, err
	}
	reply := dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:                 header.ID,
			Response:           true,
			OpCode:             header.OpCode,
			RecursionDesired:   header.RecursionDesired,
			RecursionAvailable: true,
			RCode:              dnsmessage.RCodeSuccess,
		},
		Questions: questions,
	}
	if header.OpCode != 0 {
		reply.RCode = dnsmessage.RCodeNotImplemented
		return reply.Pack()
	}
	for _, q := range questions {
		name := strings.ToLower(strings.TrimSuffix(q.Name.String(), "."))
		if q.Class != dnsmessage.ClassINET || name == "" {
			continue
		}
		if addrs, ok := f.fixed[name]; ok {
			for _, addr := range addrs {
				if (q.Type == dnsmessage.TypeA && addr.Is4()) || (q.Type == dnsmessage.TypeAAAA && addr.Is6()) {
					reply.Answers = append(reply.Answers, addrRecord(q.Name, addr))
				}
			}
			continue
		}
		if q.Type != dnsmessage.TypeA {
			continue
		}
		if name == "localhost" || strings.HasSuffix(name, ".localhost") {
			reply.Answers = append(reply.Answers, addrRecord(q.Name, netip.AddrFrom4([4]byte{127, 0, 0, 1})))
			continue
		}
		reply.Answers = append(reply.Answers, addrRecord(q.Name, f.addrFor(name)))
	}
	return reply.Pack()
}

// addrRecord is an A record for an IPv4 address, else an AAAA record.
func addrRecord(name dnsmessage.Name, addr netip.Addr) dnsmessage.Resource {
	if addr.Is4() {
		return dnsmessage.Resource{
			Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: fakeTTL},
			Body:   &dnsmessage.AResource{A: addr.As4()},
		}
	}
	return dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{Name: name, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET, TTL: fakeTTL},
		Body:   &dnsmessage.AAAAResource{AAAA: addr.As16()},
	}
}

// Target is where a connection to dst goes through the proxy: the name a
// fake address stands for, or dst itself. ok is false for a fake address
// whose name is forgotten (from before a restart or reused).
func (f *FakeDNS) Target(dst netip.AddrPort) (target string, ok bool) {
	addr := dst.Addr().Unmap()
	if !IsFake(addr) {
		return netip.AddrPortFrom(addr, dst.Port()).String(), true
	}
	name, ok := f.Name(addr)
	if !ok {
		return "", false
	}
	return net.JoinHostPort(name, strconv.Itoa(int(dst.Port()))), true
}

// vpnNet holds the VPN interface's own addresses (CoreService.kt): its
// DNS server is one of them. Nothing else listens there.
var vpnNet = netip.MustParsePrefix("172.19.0.0/30")

// filterNets hold the addresses Iran's filtering DNS gives for blocked
// names. An app may have cached one from before the VPN started; through
// the VPN the name resolves properly (FakeDNS, then Mozilla), so a
// connection to one is refused at once, and the app asks again.
var filterNets = []netip.Prefix{
	netip.MustParsePrefix("10.10.34.0/24"),
	netip.MustParsePrefix("2001:4188:2:600:10:10:34:0/120"),
}

// IsFilterAddress reports whether addr is a filtering DNS's answer.
func IsFilterAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, n := range filterNets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// IsVPNAddress reports whether addr is one of the VPN interface's own.
func IsVPNAddress(addr netip.Addr) bool { return vpnNet.Contains(addr.Unmap()) }

// ServeStream answers DNS over a stream (DNS over TCP: each message after
// its two-byte length) until the peer stops sending.
func (f *FakeDNS) ServeStream(rw io.ReadWriter) error {
	var size [2]byte
	for {
		if _, err := io.ReadFull(rw, size[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		query := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, err := io.ReadFull(rw, query); err != nil {
			return err
		}
		reply, err := f.Answer(query)
		if err != nil {
			return err
		}
		out := make([]byte, 2, 2+len(reply))
		binary.BigEndian.PutUint16(out, uint16(len(reply)))
		if _, err := rw.Write(append(out, reply...)); err != nil {
			return err
		}
	}
}
