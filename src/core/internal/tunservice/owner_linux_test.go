package tunservice

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"testing"
)

// procWord writes 4 bytes of an address as /proc/net does on this machine.
func procWord(b []byte) string { return fmt.Sprintf("%08X", binary.NativeEndian.Uint32(b)) }

func TestProcNetAddressesAreRead(t *testing.T) {
	v4 := netip.MustParseAddr("198.18.0.1").As4()
	v6 := netip.MustParseAddr("fdfe:dcba:9876::1").As16()
	var hex6 string
	for i := 0; i < 16; i += 4 {
		hex6 += procWord(v6[i : i+4])
	}
	for s, want := range map[string]string{
		procWord(v4[:]) + ":C350": "198.18.0.1:50000",
		hex6 + ":0050":            "[fdfe:dcba:9876::1]:80",
	} {
		got, ok := parseProcAddr(s)
		if !ok || got.String() != want {
			t.Errorf("parseProcAddr(%q) = %s %v, want %s", s, got, ok, want)
		}
	}
	for _, bad := range []string{"", "0100007F", "XYZ:0050", "0100007F:XYZ", "01007F:0050"} {
		if _, ok := parseProcAddr(bad); ok {
			t.Errorf("parseProcAddr(%q) read an address", bad)
		}
	}
}

func TestASocketIsFoundInATable(t *testing.T) {
	lo := netip.MustParseAddr("198.18.0.1").As4()
	any4 := [4]byte{}
	table := fmt.Sprintf(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: %s:C350 08080808:0035 01 00000000:00000000 00:00000000 00000000  1000        0 4242 1 0000000000000000 100 0 0 10 0
   1: %s:C351 00000000:0000 07 00000000:00000000 00:00000000 00000000   999        0 4343 2 0000000000000000 0
`, procWord(lo[:]), procWord(any4[:]))
	if inode, uid, ok := findSocket([]byte(table), false, netip.MustParseAddrPort("198.18.0.1:50000")); !ok || inode != 4242 || uid != 1000 {
		t.Errorf("TCP: %d %d %v", inode, uid, ok)
	}
	// A UDP socket bound to any address answers for every address.
	if inode, uid, ok := findSocket([]byte(table), true, netip.MustParseAddrPort("198.18.0.1:50001")); !ok || inode != 4343 || uid != 999 {
		t.Errorf("UDP on any address: %d %d %v", inode, uid, ok)
	}
	if _, _, ok := findSocket([]byte(table), false, netip.MustParseAddrPort("198.18.0.1:50001")); ok {
		t.Error("a TCP socket on any address was taken for one with an address")
	}
}

// The program of a connection is found by its local address: this test's
// own TCP connection and UDP socket.
func TestTheProgramOfAConnectionIsFound(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c, err := net.Dial("tcp4", ln.Addr().String())
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
		{true, netip.AddrPortFrom(netip.MustParseAddr("198.18.0.1"), u.LocalAddr().(*net.UDPAddr).AddrPort().Port())},
	} {
		path, err := ownerPath(s.udp, s.addr)
		if err != nil || path != self {
			t.Errorf("udp=%v %s: %q %v, want %q", s.udp, s.addr, path, err, self)
		}
	}
	if path, err := ownerPath(false, netip.MustParseAddrPort("198.18.0.1:1")); err == nil {
		t.Errorf("a socket nobody has was found: %q", path)
	}
}
