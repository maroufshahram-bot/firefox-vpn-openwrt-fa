package tunservice

import "io"

// On Linux the TUN's rules (tunstack) are the filters: every packet to port
// 53 goes into the TUN, whatever its address, but from a socket bound to
// an interface (the core's); systemd-resolved is given the TUN's DNS
// server for every name (sing-tun runs resolvectl); and an adapter without
// IPv6 gets an unreachable rule for IPv6 (tunstack.Config.BlockIPv6).
// Nothing is left to do here.

func startFilters(uint32, []string, bool) (io.Closer, error) { return nopCloser{}, nil }

func restoreFilters() error { return nil }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
