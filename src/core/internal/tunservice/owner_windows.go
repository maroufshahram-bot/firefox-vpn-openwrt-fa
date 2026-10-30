package tunservice

import (
	"errors"
	"net/netip"

	"github.com/sagernet/sing/common/winiphlpapi"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// ownerPath is the executable of the program whose socket has source as
// its local address: its process from Windows' table of sockets, by owner.
func ownerPath(udp bool, source netip.AddrPort) (string, error) {
	network := "tcp"
	if udp {
		network = "udp"
	}
	pid, err := winiphlpapi.FindPid(network, source)
	if err != nil {
		return "", err
	}
	// The idle process and the kernel have no executable.
	if pid == 0 || pid == 4 {
		return "", errors.New("a connection of the system itself")
	}
	return tunapi.ProcessPath(pid)
}
