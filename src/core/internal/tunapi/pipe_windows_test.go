package tunapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// testPipe gives the test a pipe name of its own.
func testPipe(t *testing.T) {
	t.Helper()
	old := pipeName
	pipeName = fmt.Sprintf(`\\.\pipe\firefox-vpn-tun-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { pipeName = old })
}

func currentUser(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid.String()
}

// listenAsUser serves the test pipe as this test's user, its owner.
func listenAsUser(t *testing.T) net.Listener {
	t.Helper()
	user := currentUser(t)
	sddl, err := securityDescriptor(user, []string{user}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := listenSDDL(sddl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// A pipe made under the name by another program (owned by a user, not the
// system) is refused.
func TestAPipeOwnedByAUserIsRefused(t *testing.T) {
	testPipe(t)
	ln := listenAsUser(t)
	go func() {
		if c, err := ln.Accept(); err == nil {
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := Dial(ctx)
	if err == nil {
		c.Close()
		t.Fatal("a user's pipe was accepted as the service")
	}
	if !strings.Contains(err.Error(), "not to the system") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNoPipeMeansNoService(t *testing.T) {
	testPipe(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Dial(ctx); !errors.Is(err, ErrNoService) {
		t.Fatalf("got %v, want ErrNoService", err)
	}
}

// A client with the access users get talks to the service, and the service
// learns which process it is.
func TestTheServiceSeesWhichProcessConnected(t *testing.T) {
	testPipe(t)
	ln := listenAsUser(t)
	served := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			served <- err
			return
		}
		defer c.Close()
		pid, err := ClientProcess(c)
		if err != nil || pid != uint32(os.Getpid()) {
			served <- fmt.Errorf("client process %d, %v; want %d", pid, err, os.Getpid())
			return
		}
		path, err := ProcessPath(pid)
		if self, _ := os.Executable(); err != nil || !strings.EqualFold(path, self) {
			served <- fmt.Errorf("client path %q, %v; want %q", path, err, self)
			return
		}
		conn := NewConn(c)
		m, err := conn.Receive()
		if err == nil {
			err = conn.Send(Message{Type: TypeHello, Version: m.Version})
		}
		served <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := winio.DialPipeAccess(ctx, pipeName, clientAccess)
	if err != nil {
		t.Fatal(err)
	}
	conn := NewConn(raw)
	defer conn.Close()
	if err := conn.Send(Hello("9.9.9")); err != nil {
		t.Fatal(err)
	}
	reply, err := conn.Receive()
	if err != nil || reply.Version != "9.9.9" {
		t.Fatalf("reply %+v, %v", reply, err)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

// Users get exactly what a client asks for: never FILE_APPEND_DATA, which
// on a pipe is the right to create another server end and take clients.
func TestUsersMayNotServeThePipe(t *testing.T) {
	if clientAccess&windows.FILE_APPEND_DATA != 0 || userAccess&windows.FILE_APPEND_DATA != 0 {
		t.Fatal("the user access includes FILE_APPEND_DATA")
	}
	// GENERIC_READ as the pipe file system maps it, and FILE_WRITE_DATA.
	if userAccess != 0x12008b {
		t.Fatalf("user access %#x", userAccess)
	}
	sddl, err := securityDescriptor("SY", nil, []string{"S-1-5-21-1-2-3-1001"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12008b;;;S-1-5-21-1-2-3-1001)"; sddl != want {
		t.Fatalf("security descriptor %s, want %s", sddl, want)
	}
	if _, err := windows.SecurityDescriptorFromString(sddl); err != nil {
		t.Fatalf("the security descriptor does not parse: %v", err)
	}
	if _, err := securityDescriptor("SY", nil, []string{"not a SID"}); err == nil {
		t.Fatal("a bad SID was accepted")
	}
}
