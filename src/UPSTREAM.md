# Upstream provenance

Project: https://github.com/armanlife/firefox-vpn-client
Commit: `9f8350ce1291b51478f037d2fab8844dee5c0f74`
Core version: `1.0.5`
License: GPL-3.0; see `LICENSE` in this directory and repository root.

`core/` is recovered byte-for-byte from this repository's v0.1.0 distribution.
The integration and installer live outside the upstream source tree. Build only
`core/cmd/firefox-vpn-core`; the desktop/TUN applications are outside this package.
The core's web interface remains upstream's interface; bilingual project docs
and installer messages do not imply a translated upstream dashboard.
