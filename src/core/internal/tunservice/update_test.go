package tunservice

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/armanlife/firefox-vpn-client/core/internal/signing"
)

// signedFile writes an executable and its signature by priv.
func signedFile(t *testing.T, data []byte, priv ed25519.PrivateKey) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "firefox-vpn-tun.exe")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))
	if err := os.WriteFile(SignatureFile(path), []byte(sig+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func withKey(t *testing.T, pub ed25519.PublicKey) {
	t.Helper()
	old := signing.Key
	signing.Key = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { signing.Key = old })
}

func TestASignedUpdateIsAccepted(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	withKey(t, pub)
	data := []byte("MZ new service")
	got, err := readSigned(signedFile(t, data, priv))
	if err != nil || string(got) != string(data) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestUpdatesNotSignedByTheKeyAreRefused(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil)
	withKey(t, pub)

	if _, err := readSigned(signedFile(t, []byte("MZ"), other)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("another key's signature: %v", err)
	}
	path := signedFile(t, []byte("MZ original"), priv)
	if err := os.WriteFile(path, []byte("MZ changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSigned(path); err == nil {
		t.Fatal("a changed file was accepted")
	}
	os.Remove(SignatureFile(path))
	if _, err := readSigned(path); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Fatalf("no signature: %v", err)
	}
}

func TestABuildWithoutAKeyTakesNoUpdates(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	withKey(t, nil)
	signing.Key = ""
	if _, err := readSigned(signedFile(t, []byte("MZ"), priv)); err == nil || !strings.Contains(err.Error(), "takes no updates") {
		t.Fatalf("got %v", err)
	}
}

func TestTheSignatureOfAMacAppsExecutableIsInItsResources(t *testing.T) {
	app := filepath.Join("Applications", "Firefox VPN.app", "Contents")
	for exe, want := range map[string]string{
		filepath.Join(app, "MacOS", "firefox-vpn-tun"): filepath.Join(app, "Resources", "firefox-vpn-tun.sig"),
		filepath.Join("opt", "firefox-vpn-tun", "x"):   filepath.Join("opt", "firefox-vpn-tun", "x.sig"),
		filepath.Join("core", "firefox-vpn-tun.exe"):   filepath.Join("core", "firefox-vpn-tun.exe.sig"),
		filepath.Join("MacOS", "firefox-vpn-tun"):      filepath.Join("MacOS", "firefox-vpn-tun.sig"),
	} {
		if got := SignatureFile(exe); got != want {
			t.Errorf("SignatureFile(%s) = %s, want %s", exe, got, want)
		}
	}
}
