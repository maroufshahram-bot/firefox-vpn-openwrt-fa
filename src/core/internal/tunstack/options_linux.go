package tunstack

import (
	"errors"

	"github.com/sagernet/netlink"
	tun "github.com/sagernet/sing-tun"
)

// The routing table and the rules' first priority of the TUN's routes on
// Linux: sing-tun puts the routes into a table of their own and sends the
// computer's traffic to it with policy rules (ip rule), which keep the
// local networks' routes, send DNS to any address into the TUN, and leave
// sockets bound to an interface (the core's, internal/egress) and replies
// to connections from outside (SSH to this computer) on the network they
// came from. Not sing-box's 2022 and 9000, so both can run.
const (
	routeTable = 2026
	ruleIndex  = 9100
)

func deviceName(name string) string { return name }

func platformOptions(o *tun.Options, cfg Config) {
	o.IPRoute2TableIndex = routeTable
	o.IPRoute2RuleIndex = ruleIndex
	// Without an IPv6 address on the device, an unreachable rule keeps
	// IPv6 from leaving outside it.
	o.StrictRoute = cfg.BlockIPv6
}

// CleanupStale removes the rules a TUN left when its process ended without
// closing it (the device and its routes go with the process; the rules
// stay, and with IPv6 blocked they would keep blocking it).
func CleanupStale() error {
	rules, err := netlink.RuleList(netlink.FAMILY_ALL)
	if err != nil {
		return err
	}
	var errs []error
	for _, r := range rules {
		if r.Priority < ruleIndex || r.Priority > ruleIndex+10 {
			continue
		}
		del := netlink.NewRule()
		del.Family = r.Family
		del.Priority = r.Priority
		if err := netlink.RuleDel(del); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
