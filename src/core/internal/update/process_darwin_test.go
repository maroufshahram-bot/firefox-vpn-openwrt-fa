package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The new version opens through Launch Services, with the app's arguments,
// not through startApp's fallback. A bundle whose executable is a script
// stands in for the app; it writes its parent (launchd's 1 when Launch
// Services started it) and arguments.
func TestTheAppOpensWithItsArguments(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "Update Test.app")
	marker := filepath.Join(t.TempDir(), "started")
	write(t, filepath.Join(bundle, "Contents", "Info.plist"), `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>run</string>
	<key>CFBundleIdentifier</key>
	<string>com.arman.firefoxvpn.updatetest</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
</dict>
</plist>
`)
	run := filepath.Join(bundle, "Contents", "MacOS", "run")
	write(t, run, "#!/bin/sh\necho \"$PPID $*\" > '"+marker+"'\n")
	if err := os.Chmod(run, 0o755); err != nil {
		t.Fatal(err)
	}
	old := thePackage
	thePackage.app = "Contents/MacOS/run"
	t.Cleanup(func() { thePackage = old })
	if err := startApp(bundle, []string{"--hidden"}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		data, err := os.ReadFile(marker)
		if err == nil && strings.HasSuffix(strings.TrimSpace(string(data)), "--hidden") {
			if parent, _, _ := strings.Cut(string(data), " "); parent != "1" {
				t.Errorf("started by process %s, not through Launch Services", parent)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the app did not start: %q %v", data, err)
		}
	}
}

// After an update the bundle no longer carries the mark of a downloaded
// app; a bundle without it is fine too.
func TestTheBundleLosesTheQuarantineMark(t *testing.T) {
	bundle := t.TempDir()
	if err := unix.Setxattr(bundle, quarantine, []byte("0081;65f00000;Safari;"), 0); err != nil {
		t.Fatal(err)
	}
	if err := setInstalledVersion(bundle, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getxattr(bundle, quarantine, nil); !errors.Is(err, unix.ENOATTR) {
		t.Fatalf("the mark stays: %v", err)
	}
	if err := setInstalledVersion(bundle, "1.2.3"); err != nil {
		t.Fatal(err)
	}
}
