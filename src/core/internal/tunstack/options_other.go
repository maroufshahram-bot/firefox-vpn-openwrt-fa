//go:build !windows && !linux && !darwin

package tunstack

import tun "github.com/sagernet/sing-tun"

func deviceName(name string) string { return name }

func platformOptions(*tun.Options, Config) {}

// CleanupStale removes what a TUN that ended without closing left.
func CleanupStale() error { return nil }
