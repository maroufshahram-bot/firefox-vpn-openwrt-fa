//go:build !windows

package tunservice

// AdapterName is the TUN device's name (Linux allows no spaces; macOS names
// it utunN itself, tunstack).
const AdapterName = "firefox-vpn"
