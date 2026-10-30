package tunservice

import (
	"net"
	"net/netip"
	"os"
	"strings"
	"testing"
)

// The program of a connection is found by its local address: this test's
// own TCP connection and UDP socket.
func TestTheProgramOfAConnectionIsFound(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	for _, s := range []struct {
		udp  bool
		addr netip.AddrPort
	}{
		{false, c.LocalAddr().(*net.TCPAddr).AddrPort()},
		// Bound to any address: found by its port.
		{true, netip.AddrPortFrom(netip.MustParseAddr("198.18.0.1"), u.LocalAddr().(*net.UDPAddr).AddrPort().Port())},
	} {
		path, err := ownerPath(s.udp, s.addr)
		if err != nil || !strings.EqualFold(path, self) {
			t.Errorf("udp=%v %s: %q %v, want %q", s.udp, s.addr, path, err, self)
		}
	}
	if path, err := ownerPath(false, netip.MustParseAddrPort("198.18.0.1:1")); err == nil {
		t.Errorf("a socket nobody has was found: %q", path)
	}
}
