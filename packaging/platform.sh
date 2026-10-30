#!/bin/sh
# Pure compatibility checks; executable probing resolves MIPS endianness.
ffvpn_arch_matches() {
    case "$1:$2" in
        armv7:armv7l|arm64:aarch64|amd64:x86_64|mips:mips|mipsle:mipsel|mipsle:mips) return 0 ;;
        *) return 1 ;;
    esac
}
ffvpn_reference_platform() {
    [ "$1" = OpenWrt ] && [ "$2" = 25.12.1 ] &&
    [ "$3" = ipq40xx/chromium ] && [ "$4" = 'Google WiFi (Gale)' ] &&
    [ "$5" = armv7l ]
}
ffvpn_supported_family() {
    case "$1" in OpenWrt|ImmortalWrt) return 0 ;; *) return 1 ;; esac
}
