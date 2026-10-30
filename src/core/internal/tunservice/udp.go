package tunservice

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/egress"
	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tun"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunstack"
)

// The UDP of a program that skips the VPN goes directly: each packet to its
// destination outside the TUN, a fake address by its name, which the
// physical network's DNS resolves; the replies come back to the app from
// the address it sent to. Replaced in tests.
var (
	listenUDPDirect = egress.ListenUDP
	lookupDirect    = egress.LookupNetIP
)

// udpLookupTimeout bounds resolving a fake address's name.
const udpLookupTimeout = 5 * time.Second

// packetFlow is the part of tunstack.PacketFlow the relay uses; tests fake
// it.
type packetFlow interface {
	Accept()
	Read(b []byte) (int, netip.AddrPort, error)
	Write(b []byte, from netip.AddrPort) error
}

// udpRelay carries one flow: one socket per address family, opened when
// the app first sends to one.
type udpRelay struct {
	sess *session
	flow packetFlow
	// names are the real addresses of the fake ones the app sent to.
	names map[netip.Addr]netip.Addr
	mu    sync.Mutex
	// back maps a real address the app's packets went to back to the one
	// it sent them to.
	back   map[netip.AddrPort]netip.AddrPort
	v4, v6 *net.UDPConn
}

// relayUDP carries a flow takeUDP took until it ends.
func (s *session) relayUDP(f *tunstack.PacketFlow) { s.relay(f) }

func (s *session) relay(f packetFlow) {
	r := &udpRelay{sess: s, flow: f, names: map[netip.Addr]netip.Addr{}, back: map[netip.AddrPort]netip.AddrPort{}}
	defer r.close()
	f.Accept()
	b := make([]byte, 65535)
	for {
		n, dst, err := f.Read(b)
		if err != nil {
			return
		}
		peer, ok := r.resolve(dst)
		if !ok {
			continue
		}
		conn, err := r.socket(peer.Addr())
		if err != nil {
			logx.Debug("TUN: no direct UDP socket for dst=%s err=%s", peer, logx.Err(err))
			continue
		}
		r.mu.Lock()
		r.back[peer] = dst
		r.mu.Unlock()
		_, _ = conn.WriteToUDPAddrPort(b[:n], peer)
	}
}

// resolve is where a packet to dst really goes: dst, or the address of the
// name a fake one stands for; not a filtering DNS's answer.
func (r *udpRelay) resolve(dst netip.AddrPort) (netip.AddrPort, bool) {
	addr := dst.Addr().Unmap()
	if isFilterAddress(addr) {
		return netip.AddrPort{}, false
	}
	if !tun.IsFake(addr) {
		return netip.AddrPortFrom(addr, dst.Port()), true
	}
	if peer, ok := r.names[addr]; ok {
		return netip.AddrPortFrom(peer, dst.Port()), true
	}
	name, ok := r.sess.dns.Name(addr)
	if !ok {
		return netip.AddrPort{}, false
	}
	ctx, cancel := context.WithTimeout(r.sess.ctx, udpLookupTimeout)
	addrs, err := lookupDirect(ctx, name)
	cancel()
	if err != nil || len(addrs) == 0 {
		logx.Debug("TUN: resolving name=%s for direct UDP failed err=%s", name, logx.Err(err))
		return netip.AddrPort{}, false
	}
	// IPv4 first: the app sent from IPv4, to a fake IPv4 address.
	peer := addrs[0].Unmap()
	for _, a := range addrs {
		if a.Unmap().Is4() {
			peer = a.Unmap()
			break
		}
	}
	r.names[addr] = peer
	return netip.AddrPortFrom(peer, dst.Port()), true
}

// socket is the flow's socket for addr's family.
func (r *udpRelay) socket(addr netip.Addr) (*net.UDPConn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	slot := &r.v4
	if addr.Is6() {
		slot = &r.v6
	}
	if *slot != nil {
		return *slot, nil
	}
	conn, err := listenUDPDirect(addr)
	if err != nil {
		return nil, err
	}
	*slot = conn
	go r.readBack(conn)
	return conn, nil
}

// readBack writes each reply to the app, from the address it sent to.
func (r *udpRelay) readBack(conn *net.UDPConn) {
	b := make([]byte, 65535)
	for {
		n, from, err := conn.ReadFromUDPAddrPort(b)
		if err != nil {
			return
		}
		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())
		r.mu.Lock()
		to, ok := r.back[from]
		r.mu.Unlock()
		if !ok {
			// A peer the app never sent to (a call's other end): as it is.
			to = from
		}
		_ = r.flow.Write(b[:n], to)
	}
}

func (r *udpRelay) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range []*net.UDPConn{r.v4, r.v6} {
		if c != nil {
			_ = c.Close()
		}
	}
}
