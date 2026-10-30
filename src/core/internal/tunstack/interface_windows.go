package tunstack

import (
	"errors"

	tun "github.com/sagernet/sing-tun"
	"golang.org/x/sys/windows"
)

// The adapter's description names this program, not sing-tun.
func init() { tun.TunnelType = "Firefox VPN" }

func deviceName(name string) string { return name }

// platformOptions: the service's own filters (internal/wfp) keep DNS in
// and IPv6 out, not sing-tun's strict route.
func platformOptions(*tun.Options, Config) {}

// CleanupStale removes what a TUN that ended without closing left; on
// Windows the adapter and the filters go with the process.
func CleanupStale() error { return nil }

// Interface is the adapter's name, interface index, and LUID.
func (d *Device) Interface() (name string, index uint32, luid uint64, err error) {
	l, ok := d.tun.(interface{ LUID() uint64 })
	if !ok {
		return "", 0, 0, errors.New("tunstack: the adapter has no LUID")
	}
	row := windows.MibIfRow2{InterfaceLuid: l.LUID()}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormal, &row); err != nil {
		return "", 0, 0, err
	}
	return d.cfg.Name, row.InterfaceIndex, row.InterfaceLuid, nil
}
