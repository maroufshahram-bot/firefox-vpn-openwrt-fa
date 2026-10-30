package tunapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"

	"golang.org/x/sys/unix"
)

// SocketPath is the service's socket, in a folder only root may write to.
const SocketPath = "/var/run/firefox-vpn-tun.sock"

// LaunchdLabel is the service's launchd job.
const LaunchdLabel = "com.arman.firefoxvpn.tun"

// ServiceFile is the service's launchd job file.
const ServiceFile = "/Library/LaunchDaemons/" + LaunchdLabel + ".plist"

// peer asks the kernel who is at the other end of c.
func peer(c net.Conn) (peerInfo, error) {
	var p peerInfo
	err := fdOf(c, func(fd int) error {
		cred, err := unix.GetsockoptXucred(fd, unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if err != nil {
			return err
		}
		pid, err := unix.GetsockoptInt(fd, unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		if err != nil {
			return err
		}
		p = peerInfo{uid: cred.Uid, pid: int32(pid)}
		return nil
	})
	return p, err
}

// ProcessPath is the executable of process pid: the path the kernel keeps
// before its arguments (kern.procargs2: argc, then the path).
func ProcessPath(pid uint32) (string, error) {
	data, err := unix.SysctlRaw("kern.procargs2", int(pid))
	if err != nil {
		return "", err
	}
	if len(data) < 4 {
		return "", errors.New("no process arguments")
	}
	_ = binary.LittleEndian.Uint32(data[:4]) // argc
	path, _, ok := bytes.Cut(data[4:], []byte{0})
	if !ok || len(path) == 0 {
		return "", errors.New("no executable path")
	}
	return string(path), nil
}
