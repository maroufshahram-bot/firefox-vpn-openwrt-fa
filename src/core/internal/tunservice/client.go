package tunservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// The commands an ordinary user runs: the app (installing on the first TUN
// use), Setup (installing and uninstalling), or a person in a terminal.
// Only installing and uninstalling need administrator rights, and they ask
// for them, in an elevated copy of this executable, once: Windows' prompt,
// polkit's on Linux (pkexec), the administrator password on macOS
// (client_*.go).

// ErrCancelled is the user declining the administrator prompt.
var ErrCancelled = errors.New("the administrator permission was not given")

// UpdateService asks the running service to replace itself with the
// signed executable exe.
func UpdateService(exe string) error {
	return askService(tunapi.Message{Type: tunapi.TypeUpdate, Path: exe}, tunapi.TypeUpdating)
}

// askService says hello, sends m, and expects the reply type want.
func askService(m tunapi.Message, want string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := tunapi.Dial(ctx)
	if err != nil {
		return err
	}
	c := tunapi.NewConn(raw)
	defer c.Close()
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	if err := c.Send(tunapi.Hello(version.Current)); err != nil {
		return err
	}
	// Any reply to hello will do (update and uninstall need no common
	// protocol), but a refusal.
	reply, err := c.Receive()
	if err != nil {
		return err
	}
	if e, ok := tunapi.AsError(reply).(*tunapi.Error); ok && e.Code == tunapi.CodeNotAllowed {
		return tunapi.ErrNotAllowed
	}
	if err := c.Send(m); err != nil {
		return err
	}
	reply, err = c.Receive()
	if err != nil {
		return err
	}
	if err := tunapi.AsError(reply); err != nil {
		return err
	}
	if reply.Type != want {
		return fmt.Errorf("unexpected reply %q", reply.Type)
	}
	return nil
}

func waitRemoved(limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		installed, err := tunapi.Installed()
		if err == nil && !installed {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the service is still installed")
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// WriteReport writes err for the process that started this elevated one,
// to the file it made: opened without creating it, which Linux refuses root
// for another user's file in /tmp (fs.protected_regular).
func WriteReport(path string, err error) {
	if path == "" || err == nil {
		return
	}
	f, ferr := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if ferr != nil {
		return
	}
	_, _ = f.WriteString(err.Error())
	f.Close()
}

// readReport is what the elevated copy wrote to the report file.
func readReport(path string) string {
	text, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(text))
}

// waitForService waits until the service answers.
func waitForService(limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c, err := tunapi.Dial(ctx)
		cancel()
		if err == nil {
			c.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the service does not answer: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
