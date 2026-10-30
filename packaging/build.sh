#!/bin/sh
# Build on a development machine, never on the router.
set -eu
base=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
target=${1:-armv7}
case "$target" in
    armv7) arch=arm; arm=7 ;;
    arm64) arch=arm64; arm= ;;
    amd64) arch=amd64; arm= ;;
    mips) arch=mips; arm= ;;
    mipsle) arch=mipsle; arm= ;;
    *) echo 'Target must be armv7, arm64, amd64, mips or mipsle.' >&2; exit 2 ;;
esac
out="$base/build/$target"
mkdir -p "$out/bin"
cd "$base/src/core"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOARM="$arm" GOMIPS=softfloat \
 go build -mod=readonly -trimpath -buildvcs=false \
 -ldflags '-s -w -X github.com/armanlife/firefox-vpn-client/core/internal/version.Current=1.0.5' \
 -o "$out/bin/firefox-vpn-core" ./cmd/firefox-vpn-core
printf '%s\n' "$target" > "$out/TARGET"
cp "$base/VERSION" "$out/PACKAGE_VERSION"
cp "$base/packaging/install.sh" "$base/packaging/uninstall.sh" \
 "$base/packaging/platform.sh" "$base/packaging/diagnose.sh" "$out/"
cp "$base/LICENSE" "$base/README.md" "$base/README.fa.md" \
 "$base/README.ru.md" "$base/README.ar.md" "$out/"
mkdir -p "$out/docs"
cp "$base"/docs/*.md "$out/docs/"
cp -R "$base/src" "$out/"
cd "$out"
sha256sum bin/firefox-vpn-core > SHA256SUMS
echo "Built $target. Package it with: python3 scripts/package.py $target"

