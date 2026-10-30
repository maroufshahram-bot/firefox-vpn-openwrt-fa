// Package signing checks the Ed25519 signatures of updates: the releases'
// SHA256SUMS.txt, which the Windows app checks before it installs a new
// version of itself (internal/update), and the TUN service's executable,
// which the installed service checks before it replaces itself
// (internal/tunservice). tools/sign makes the key and the signatures.
package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
)

// Key is the base64 Ed25519 public key that updates must be signed with,
// set by the build:
//
//	-ldflags "-X github.com/armanlife/firefox-vpn-client/core/internal/signing.Key=..."
//
// The release build uses the release key, whose private half only CI holds
// (the secret FIREFOX_VPN_SIGNING_KEY); a local build may use a developer
// key. A build without one takes no updates.
var Key string

// ErrNoKey means this build has no key, so it takes no updates.
var ErrNoKey = errors.New("this build has no update key, so it takes no updates")

// Available reports whether this build can check signatures.
func Available() bool { return Key != "" }

// Verify checks sig, a base64 signature as tools/sign writes it (one line),
// over data against Key.
func Verify(data, sig []byte) error {
	if Key == "" {
		return ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(Key)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("the build's update key is broken")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("the signature is unreadable")
	}
	if !ed25519.Verify(ed25519.PublicKey(key), data, raw) {
		return errors.New("the signature does not match")
	}
	return nil
}
