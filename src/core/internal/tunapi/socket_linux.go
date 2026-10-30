package tunapi

import (
	"net"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// SocketPath is the service's socket, in a folder only root may write to.
const SocketPath = "/run/firefox-vpn-tun.sock"

// ServiceFile is the service's systemd unit.
const ServiceFile = "/etc/systemd/system/" + ServiceName + ".service"

// peer asks the kernel who is at the other end of c.
func peer(c net.Conn) (peerInfo, error) {
	var p peerInfo
	err := fdOf(c, func(fd int) error {
		cred, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			return err
		}
		p = peerInfo{uid: cred.Uid, pid: cred.Pid}
		return nil
	})
	return p, err
}

// ProcessPath is the executable of process pid.
func ProcessPath(pid uint32) (string, error) {
	return os.Readlink("/proc/" + strconv.FormatUint(uint64(pid), 10) + "/exe")
}
