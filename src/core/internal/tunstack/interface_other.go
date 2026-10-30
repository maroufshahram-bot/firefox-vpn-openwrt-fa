//go:build !windows

package tunstack

import "net"

// Interface is the adapter's name and interface index; the LUID is
// Windows'.
func (d *Device) Interface() (name string, index uint32, luid uint64, err error) {
	name, err = d.tun.Name()
	if err != nil {
		return "", 0, 0, err
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", 0, 0, err
	}
	return name, uint32(iface.Index), 0, nil
}
