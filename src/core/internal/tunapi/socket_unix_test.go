//go:build linux || darwin

package tunapi

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// useTestSocket serves the socket in a folder of the test's, as this user.
// It is short: a socket's path has a limit (104 bytes on macOS).
func useTestSocket(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "tun")
	if err != nil {
		t.Fatal(err)
	}
	oldPath, oldUID := socketPath, serverUID
	socketPath, serverUID = filepath.Join(dir, "s"), os.Getuid()
	t.Cleanup(func() {
		socketPath, serverUID = oldPath, oldUID
		os.RemoveAll(dir)
	})
}

// accepted dials the listener and returns the connection it accepted.
func accepted(t *testing.T, ln net.Listener) (server, client net.Conn) {
	t.Helper()
	got := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			t.Error(err)
		}
		got <- c
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	server = <-got
	if server == nil {
		t.FailNow()
	}
	t.Cleanup(func() { server.Close() })
	return server, client
}

func TestTheSocketTellsWhoConnected(t *testing.T) {
	useTestSocket(t)
	ln, err := Listen([]string{strconv.Itoa(os.Getuid())})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if st, err := os.Stat(socketPath); err != nil || st.Mode().Perm() != 0o666 {
		t.Fatalf("the socket's mode is %v (%v), want every user's", st.Mode(), err)
	}
	c, _ := accepted(t, ln)
	if !Allowed(c) {
		t.Fatal("the user who installed it is not allowed")
	}
	pid, err := ClientProcess(c)
	if err != nil || pid != uint32(os.Getpid()) {
		t.Fatalf("client process %d (%v), want %d", pid, err, os.Getpid())
	}
	path, err := ProcessPath(pid)
	self, _ := os.Executable()
	if err != nil || filepath.Base(path) != filepath.Base(self) {
		t.Fatalf("process path %q (%v), want %q", path, err, self)
	}
}

func TestAUserWhoDidNotInstallItIsNotAllowed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root is always allowed")
	}
	useTestSocket(t)
	ln, err := Listen([]string{strconv.Itoa(os.Getuid() + 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if c, _ := accepted(t, ln); Allowed(c) {
		t.Fatal("another user was allowed")
	}
	if _, err := Listen([]string{"someone"}); err == nil {
		t.Fatal("a bad user id was taken")
	}
}

// Only root may serve the socket: a program of another user standing in
// for the service is refused.
func TestASocketServedByAnotherUserIsRefused(t *testing.T) {
	useTestSocket(t)
	ln, err := Listen(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer c.Close()
			time.Sleep(time.Second)
		}
	}()
	serverUID = os.Getuid() + 1
	if c, err := Dial(context.Background()); err == nil {
		c.Close()
		t.Fatal("a socket of another user was trusted")
	}
}

func TestNoServiceIsErrNoService(t *testing.T) {
	useTestSocket(t)
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNoService) {
		t.Fatalf("no socket: %v", err)
	}
	// A socket left by a service that ended.
	ln, err := Listen(nil)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*listener).Listener.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("the socket file is gone: %v", err)
	}
	if _, err := Dial(context.Background()); !errors.Is(err, ErrNoService) {
		t.Fatalf("a dead socket: %v", err)
	}
	// The next service replaces it.
	ln, err = Listen(nil)
	if err != nil {
		t.Fatalf("a left socket was not replaced: %v", err)
	}
	ln.Close()
}
