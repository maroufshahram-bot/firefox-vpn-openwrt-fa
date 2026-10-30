package tunstack

import tun "github.com/sagernet/sing-tun"

// deviceName is a free utunN: macOS names its TUN devices itself.
func deviceName(string) string { return tun.CalculateInterfaceName("") }

// platformOptions: the service keeps DNS in and IPv6 out itself
// (internal/tunservice, filters_darwin.go); sing-tun routes the address
// space into the device in ranges more specific than the default route.
func platformOptions(*tun.Options, Config) {}

// CleanupStale: the device and its routes go with the process.
func CleanupStale() error { return nil }
