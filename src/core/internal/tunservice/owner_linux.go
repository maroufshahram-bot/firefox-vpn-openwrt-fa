package tunservice

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// ownerPath is the executable of the program whose socket has source as
// its local address: the socket's inode from /proc/net, then the process
// that holds it (the service runs as root, so it may look at every one).
func ownerPath(udp bool, source netip.AddrPort) (string, error) {
	inode, uid, err := socketInode(udp, source)
	if err != nil {
		return "", err
	}
	pid, err := socketOwner(inode, uid)
	if err != nil {
		return "", err
	}
	return tunapi.ProcessPath(uint32(pid))
}

// procNet is where the kernel lists the sockets; tests replace it.
var procNet = "/proc/net"

// socketInode finds the socket bound to source in the kernel's tables:
// the IPv6 one too for an IPv4 address, which a dual-stack app's socket
// has as a mapped one.
func socketInode(udp bool, source netip.AddrPort) (inode uint64, uid uint32, err error) {
	proto := "tcp"
	if udp {
		proto = "udp"
	}
	tables := []string{proto + "6"}
	if source.Addr().Unmap().Is4() {
		tables = []string{proto, proto + "6"}
	}
	for _, table := range tables {
		data, err := os.ReadFile(procNet + "/" + table)
		if err != nil {
			continue
		}
		if inode, uid, ok := findSocket(data, udp, source); ok {
			return inode, uid, nil
		}
	}
	return 0, 0, errors.New("no socket has this address")
}

// findSocket reads a table of /proc/net for the socket bound to source;
// for UDP also one bound to any address on its port.
func findSocket(table []byte, udp bool, source netip.AddrPort) (inode uint64, uid uint32, ok bool) {
	want := source.Addr().Unmap()
	for i, line := range bytes.Split(table, []byte("\n")) {
		f := strings.Fields(string(line))
		if i == 0 || len(f) < 10 {
			continue
		}
		local, ok := parseProcAddr(f[1])
		if !ok || local.Port() != source.Port() {
			continue
		}
		if addr := local.Addr().Unmap(); addr != want && !(udp && addr.IsUnspecified()) {
			continue
		}
		u, err := strconv.ParseUint(f[7], 10, 32)
		if err != nil {
			continue
		}
		n, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || n == 0 {
			continue
		}
		return n, uint32(u), true
	}
	return 0, 0, false
}

// parseProcAddr reads an address as /proc/net writes it, "0100007F:1F90":
// each 32-bit word of the address in hex as this machine's byte order has
// it, then the port in hex.
func parseProcAddr(s string) (netip.AddrPort, bool) {
	hexAddr, hexPort, ok := strings.Cut(s, ":")
	if !ok {
		return netip.AddrPort{}, false
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	raw, err := hex.DecodeString(hexAddr)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.AddrPort{}, false
	}
	b := make([]byte, len(raw))
	for i := 0; i < len(raw); i += 4 {
		binary.NativeEndian.PutUint32(b[i:], binary.BigEndian.Uint32(raw[i:]))
	}
	addr, _ := netip.AddrFromSlice(b)
	return netip.AddrPortFrom(addr, uint16(port)), true
}

// procDir is the processes' folder; tests replace it.
var procDir = "/proc"

// socketOwner is the process with a descriptor that is the socket: first
// among the processes of the socket's user, then the others (a process
// that is not dumpable shows as root's).
func socketOwner(inode uint64, uid uint32) (int, error) {
	link := "socket:[" + strconv.FormatUint(inode, 10) + "]"
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return 0, err
	}
	var others []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if info, err := e.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid != uid {
				others = append(others, e.Name())
				continue
			}
		}
		if holds(e.Name(), link) {
			return strconv.Atoi(e.Name())
		}
	}
	for _, pid := range others {
		if holds(pid, link) {
			return strconv.Atoi(pid)
		}
	}
	return 0, errors.New("no process holds the socket")
}

// holds reports whether process pid has a descriptor that is link.
func holds(pid, link string) bool {
	dir := procDir + "/" + pid + "/fd"
	fds, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		if target, err := os.Readlink(dir + "/" + fd.Name()); err == nil && target == link {
			return true
		}
	}
	return false
}
