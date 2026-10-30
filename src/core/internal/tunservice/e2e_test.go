//go:build e2e && (linux || darwin)

package tunservice

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/egress"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// TestEndToEnd runs a TUN for real, on a computer whose network it may
// change for a minute (CI's): as root, with
//
//	go test -c -tags 'e2e with_gvisor' -o e2e.test ./internal/tunservice
//	sudo ./e2e.test -test.run TestEndToEnd -test.v
//
// The service runs in this process, with a stand-in for the core: a
// SOCKS5 server on loopback whose own connections leave outside the TUN
// through egress, as the core's do. Apps' connections (curl) must reach
// it, by name through the system's DNS, which the TUN answers; and once
// the TUN is closed the routes and the DNS settings must be as before.
func TestEndToEnd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	stateDir = t.TempDir()
	before := networkState(t)
	proxy, seen := standIn(t)

	self, _ := os.Executable()
	s := NewServer(Config{Version: "9.9.9", Self: self})
	s.allowed = func(net.Conn) bool { return true }
	s.clientPath = func(net.Conn) (string, error) { return self, nil }
	a, b := net.Pipe()
	go s.serveConn(b)
	c := tunapi.NewConn(a)
	defer c.Close()
	// refused are the programs whose UDP the service said it refused.
	var refused []string
	receive := func(deadline time.Duration) (tunapi.Message, error) {
		_ = a.SetDeadline(time.Now().Add(deadline))
		for {
			m, err := c.Receive()
			if err != nil || m.Type != tunapi.TypeUDPRefused {
				return m, err
			}
			refused = append(refused, m.Program)
		}
	}
	ask := func(m tunapi.Message, want string) tunapi.Message {
		t.Helper()
		_ = a.SetDeadline(time.Now().Add(time.Minute))
		if err := c.Send(m); err != nil {
			t.Fatal(err)
		}
		reply, err := receive(time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if reply.Type != want {
			t.Fatalf("asked %s, got %+v", m.Type, reply)
		}
		return reply
	}
	ask(tunapi.Hello("9.9.9"), tunapi.TypeHello)
	opened := ask(tunapi.Message{Type: tunapi.TypeOpen, Proxy: proxy, Username: "u", Password: "p"}, tunapi.TypeOpened)
	iface := opened.Interface
	t.Logf("adapter %s, index %d", iface.Name, iface.Index)
	egress.SetTun(iface.Index, iface.LUID)
	defer egress.ClearTun()
	ask(tunapi.Message{Type: tunapi.TypeRoute}, tunapi.TypeRouted)
	// systemd-resolved is told in the background; macOS' resolver caches.
	time.Sleep(3 * time.Second)
	if runtime.GOOS == "darwin" {
		sh(t, "dscacheutil -flushcache; killall -HUP mDNSResponder")
	}
	t.Logf("routes:\n%s", sh(t, routesCommand(iface.Name)))
	t.Logf("DNS:\n%s", sh(t, dnsCommand(iface.Name)))

	// The system's DNS is the TUN's: a fake address for any name, which
	// leads a connection to the stand-in with the name. The name need not
	// exist.
	random := make([]byte, 4)
	_, _ = rand.Read(random)
	name := "e2e-" + hex.EncodeToString(random) + ".example.com"
	if fake := systemLookup(t, name); !strings.HasPrefix(fake, "198.18.") {
		t.Errorf("the system's DNS gave %q for %s, not a fake address of the TUN's", fake, name)
	}
	// Apps by name, where they ask that DNS: always on macOS; on Linux
	// through systemd-resolved's stub (WSL, for one, sends DNS to Windows
	// instead).
	if appsAskTheTun(t) {
		sh(t, "curl -s -m 15 -o /dev/null http://"+name+"/")
		if !seen.has(name + ":80") {
			t.Errorf("a connection to %s did not reach the stand-in by name; it saw %v", name, seen.list())
		}
	} else {
		t.Log("the system's resolv.conf is not systemd-resolved's stub: apps by name are not checked")
	}
	// By address, and through to the internet by name: the stand-in's own
	// connection leaves outside the TUN.
	code := sh(t, "curl -sS -m 20 -o /dev/null -w '%{http_code}' https://1.1.1.1/")
	if !seen.has("1.1.1.1:443") || (code != "200" && code != "301") {
		t.Errorf("https://1.1.1.1/ gave %q; the stand-in saw %v", code, seen.list())
	}
	fake := systemLookup(t, "example.com")
	code = sh(t, "curl -sS -m 20 -o /dev/null -w '%{http_code}' --resolve example.com:443:"+fake+" https://example.com/")
	if code != "200" || !seen.has("example.com:443") {
		t.Errorf("https://example.com/ through %s gave %q; the stand-in saw %v", fake, code, seen.list())
	}
	if n := len(seen.list()); n > 20 {
		t.Errorf("the stand-in saw %d connections: its own go back into the TUN", n)
	}

	// Programs that skip the VPN: curl's TCP connects directly, so the
	// stand-in sees nothing of it.
	ask(tunapi.Message{Type: tunapi.TypePrograms, Programs: []tunapi.Program{{Name: "curl"}}}, tunapi.TypeProgramsSet)
	through := len(seen.list())
	code = sh(t, "curl -sS -m 20 -o /dev/null -w '%{http_code}' https://1.1.1.1/")
	if (code != "200" && code != "301") || len(seen.list()) != through {
		t.Errorf("curl directly gave %q; the stand-in saw %v", code, seen.list()[through:])
	}
	// A call (UDP to a STUN port) of python3 is refused at once, and the
	// service says so; with python3's UDP skipping the VPN it is not.
	if python := sh(t, "readlink -f \"$(command -v python3)\""); python != "" {
		script := filepath.Join(t.TempDir(), "call.py")
		if err := os.WriteFile(script, []byte(callScript), 0o644); err != nil {
			t.Fatal(err)
		}
		if out := sh(t, "python3 "+script); out != "refused" {
			t.Errorf("a call through the TUN gave %q, want refused", out)
		}
		_, _ = receive(2 * time.Second)
		if want := filepath.Base(python); !slices.Contains(refused, want) {
			t.Errorf("the service told of %v, want %s", refused, want)
		}
		ask(tunapi.Message{Type: tunapi.TypePrograms, Programs: []tunapi.Program{{Name: "curl"}, {Name: filepath.Base(python), UDPOnly: true}}}, tunapi.TypeProgramsSet)
		if out := sh(t, "python3 "+script); out == "refused" {
			t.Errorf("a call of a program whose UDP skips the VPN was refused")
		} else {
			t.Logf("a call sent directly: %s", out)
		}
	} else {
		t.Log("no python3: UDP is not checked")
	}

	ask(tunapi.Message{Type: tunapi.TypeClose}, tunapi.TypeClosed)
	egress.ClearTun()
	time.Sleep(2 * time.Second)
	if after := networkState(t); after != before {
		t.Errorf("the network is not as before the TUN:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if left := sh(t, routesCommand(iface.Name)); strings.Contains(left, iface.Name) {
		t.Errorf("routes through %s are left:\n%s", iface.Name, left)
	}
}

// callScript sends a STUN binding request, as a call starts, and prints
// how it ended: "refused" (ICMP unreachable), "reply", or "timeout".
const callScript = `import os, socket
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(3)
s.connect(("1.1.1.1", 3478))
try:
    s.send(b"\x00\x01\x00\x00\x21\x12\xa4\x42" + os.urandom(12))
    s.recv(1500)
    print("reply")
except ConnectionRefusedError:
    print("refused")
except socket.timeout:
    print("timeout")
`

// systemLookup is the IPv4 address the system's resolver gives for name:
// mDNSResponder's on macOS, systemd-resolved's on Linux.
func systemLookup(t *testing.T, name string) string {
	if runtime.GOOS == "darwin" {
		return sh(t, "dscacheutil -q host -a name "+name+" | awk '/^ip_address:/ {print $2; exit}'")
	}
	return sh(t, "resolvectl query --legend=no -4 "+name+" 2>&1 | awk '{print $2; exit}'")
}

// appsAskTheTun reports whether apps' DNS goes to the system's resolver,
// which the TUN's DNS server answers.
func appsAskTheTun(t *testing.T) bool {
	if runtime.GOOS == "darwin" {
		return true
	}
	return strings.Contains(sh(t, "grep '^nameserver' /etc/resolv.conf"), "127.0.0.53")
}

func routesCommand(tun string) string {
	if runtime.GOOS == "darwin" {
		return "netstat -rn | grep -w " + tun + " || true"
	}
	return "ip rule; ip -6 rule; ip route show table 2026; ip -6 route show table 2026"
}

func dnsCommand(tun string) string {
	if runtime.GOOS == "darwin" {
		return "scutil --dns | grep -m 5 nameserver"
	}
	return "resolvectl status " + tun + " | head -8"
}

// networkState is what the TUN changes and must put back: the default
// routes, the rules (Linux), and the DNS settings.
func networkState(t *testing.T) string {
	if runtime.GOOS == "darwin" {
		return sh(t, `netstat -rn -f inet | awk '$1=="default"'; netstat -rn -f inet6 | awk '$1=="default"'; `+
			`networksetup -listallnetworkservices | tail -n +2 | tr -d '*' | while read -r s; do echo "$s: $(networksetup -getdnsservers "$s" | tr '\n' ' ')"; done`)
	}
	return sh(t, "ip route show default; ip -6 route show default; ip rule; ip -6 rule; cat /etc/resolv.conf")
}

func sh(t *testing.T, command string) string {
	t.Helper()
	out, _ := exec.Command("sh", "-c", command).CombinedOutput()
	return strings.TrimSpace(string(out))
}

type targets struct {
	mu   sync.Mutex
	seen []string
}

func (s *targets) add(target string) {
	s.mu.Lock()
	s.seen = append(s.seen, target)
	s.mu.Unlock()
}

func (s *targets) list() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *targets) has(target string) bool {
	for _, x := range s.list() {
		if x == target {
			return true
		}
	}
	return false
}

// standIn serves SOCKS5 (user u, password p) on loopback like the core's
// TUN listener, connecting through egress.
func standIn(t *testing.T) (string, *targets) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	seen := &targets{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSOCKS(c, seen)
		}
	}()
	return ln.Addr().String(), seen
}

func serveSOCKS(c net.Conn, seen *targets) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Minute))
	b := make([]byte, 256)
	read := func(n int) []byte {
		if _, err := io.ReadFull(c, b[:n]); err != nil {
			return nil
		}
		return b[:n]
	}
	// Greeting, then the user and password.
	if h := read(2); h == nil || read(int(h[1])) == nil {
		return
	}
	c.Write([]byte{5, 2})
	if h := read(2); h == nil || read(int(h[1])) == nil {
		return
	}
	if l := read(1); l == nil || read(int(l[0])) == nil {
		return
	}
	c.Write([]byte{1, 0})
	req := read(4)
	if req == nil {
		return
	}
	var host string
	switch req[3] {
	case 1:
		host = net.IP(append([]byte(nil), read(4)...)).String()
	case 3:
		l := read(1)
		if l == nil {
			return
		}
		host = string(read(int(l[0])))
	case 4:
		host = net.IP(append([]byte(nil), read(16)...)).String()
	default:
		return
	}
	p := read(2)
	if p == nil {
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p))))
	seen.add(target)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	up, err := egress.DialContext(ctx, "tcp", target)
	cancel()
	if err != nil {
		c.Write([]byte{5, 4, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer up.Close()
	c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	_ = c.SetDeadline(time.Time{})
	done := make(chan struct{}, 2)
	go func() { io.Copy(up, c); done <- struct{}{} }()
	go func() { io.Copy(c, up); done <- struct{}{} }()
	<-done
}
