package tunservice

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// InstallDir is the installed service's folder.
const InstallDir = "/Library/Application Support/Firefox VPN TUN"

// jobTarget is the service in launchctl's commands.
const jobTarget = "system/" + tunapi.LaunchdLabel

// job is the service's launchd job: started at boot, and again whenever it
// ends (after a crash, and after an update, which ends it); an uninstall
// boots it out (stopForGood).
const job = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>service</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ThrottleInterval</key>
	<integer>2</integer>
</dict>
</plist>
`

func initSystemPresent() error { return nil }

// register writes the job for the executable exe and loads it, which
// starts it.
func register(exe string) error {
	if err := replaceFile(tunapi.ServiceFile, []byte(fmt.Sprintf(job, tunapi.LaunchdLabel, xmlText(exe))), 0o644); err != nil {
		return err
	}
	// A job that was disabled (launchctl disable) does not load.
	_ = run("launchctl", "enable", jobTarget)
	var err error
	for range 5 {
		if err = run("launchctl", "bootstrap", "system", tunapi.ServiceFile); err == nil || loaded() {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

// unregister removes the job's file; it does not stop the service, which
// may be the caller.
func unregister() error {
	if err := os.Remove(tunapi.ServiceFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// stopInstalled unloads the job, which stops the service, and waits for it.
func stopInstalled() error {
	if !loaded() {
		return nil
	}
	err := run("launchctl", "bootout", jobTarget)
	deadline := time.Now().Add(30 * time.Second)
	for loaded() {
		if time.Now().After(deadline) {
			if err == nil {
				err = errors.New("the service does not stop")
			}
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

// stopForGood unloads the job, whose file is removed, so launchd does not
// start it again; launchd ends this process meanwhile (SIGTERM, no longer
// caught).
func stopForGood() {
	signal.Reset(syscall.SIGTERM, syscall.SIGINT)
	_ = run("launchctl", "bootout", jobTarget)
}

// loaded reports whether launchd has the job.
func loaded() bool {
	return exec.Command("launchctl", "print", jobTarget).Run() == nil
}

// xmlText escapes s for a plist's string.
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
