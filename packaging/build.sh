#!/bin/sh
set -eu
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$base/upstream/core"
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags '-s -w -X github.com/armanlife/firefox-vpn-client/core/internal/version.Current=1.0.5' -o "$base/bin/firefox-vpn-core" ./cmd/firefox-vpn-core
cd "$base"
sha256sum bin/firefox-vpn-core > SHA256SUMS
