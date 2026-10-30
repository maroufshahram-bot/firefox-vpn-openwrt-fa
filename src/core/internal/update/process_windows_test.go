package update

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The version in Windows' list changes for Setup's own folder only; the
// test uses a key of its own, never the app's.
func TestTheInstalledVersionIsSetForSetupsFolderOnly(t *testing.T) {
	random := make([]byte, 6)
	rand.Read(random)
	old := uninstallKey
	uninstallKey = `Software\FirefoxVPNUpdateTest-` + hex.EncodeToString(random)
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, uninstallKey)
		uninstallKey = old
	})
	// No entry: a portable copy.
	if err := setInstalledVersion(`C:\Apps\Firefox VPN Client`, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKey, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	k.SetStringValue("InstallLocation", `C:\Apps\Firefox VPN Client\`)
	k.SetStringValue("DisplayVersion", "1.0.0")
	version := func() string {
		v, _, _ := k.GetStringValue("DisplayVersion")
		return v
	}
	if err := setInstalledVersion(`D:\Portable\Firefox VPN`, "1.2.3"); err != nil || version() != "1.0.0" {
		t.Fatalf("another folder: %v, %q", err, version())
	}
	if err := setInstalledVersion(`c:\apps\firefox vpn client`, "1.12.3"); err != nil || version() != "1.12.3" {
		t.Fatalf("Setup's folder: %v, %q", err, version())
	}
	for name, want := range map[string]uint64{"MajorVersion": 1, "VersionMajor": 1, "MinorVersion": 12, "VersionMinor": 12} {
		if got, _, err := k.GetIntegerValue(name); err != nil || got != want {
			t.Errorf("%s = %d, %v; want %d", name, got, err, want)
		}
	}
}
