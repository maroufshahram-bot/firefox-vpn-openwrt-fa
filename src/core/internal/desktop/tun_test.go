package desktop

import (
	"context"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/config"
	"github.com/armanlife/firefox-vpn-client/core/internal/service"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// fakeHost is the daemon as tunSync sees it, recording what it was told.
type fakeHost struct {
	wanted   atomic.Bool
	mu       sync.Mutex
	log      []string
	status   service.TunStatus
	programs []config.TunProgram
	refused  []string
}

func (h *fakeHost) TunPrograms() []config.TunProgram {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.programs)
}
func (h *fakeHost) setPrograms(p ...config.TunProgram) {
	h.mu.Lock()
	h.programs = p
	h.mu.Unlock()
}
func (h *fakeHost) NoteRefusedUDP(program, path string) {
	h.mu.Lock()
	h.refused = append(h.refused, program+" "+path)
	h.mu.Unlock()
}
func (h *fakeHost) refusals() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.refused)
}

func (h *fakeHost) note(s string) {
	h.mu.Lock()
	h.log = append(h.log, s)
	h.mu.Unlock()
}
func (h *fakeHost) TunWanted() bool { return h.wanted.Load() }
func (h *fakeHost) OpenTunListener() (string, string, string, error) {
	h.note("listen")
	return "127.0.0.1:5555", "tun", "secret", nil
}
func (h *fakeHost) CloseTunListener()  { h.note("unlisten") }
func (h *fakeHost) ReconnectUpstream() { h.note("reconnect") }
func (h *fakeHost) SetTunStatus(s service.TunStatus) {
	h.mu.Lock()
	h.status = s
	h.mu.Unlock()
}
func (h *fakeHost) current() service.TunStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}
func (h *fakeHost) steps() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.log, " ")
}

// fakeTunService serves each dial with the next handler; with none queued
// there is no service.
type fakeTunService struct{ handlers chan func(*tunapi.Conn) }

func (s *fakeTunService) dial(context.Context) (net.Conn, error) {
	select {
	case h := <-s.handlers:
		a, b := net.Pipe()
		go func() {
			c := tunapi.NewConn(b)
			defer c.Close()
			h(c)
		}()
		return a, nil
	default:
		return nil, tunapi.ErrNoService
	}
}

// setup replaces the service, the binding, and the waits; host's log
// gets "bind index" and "unbind" too.
func setup(t *testing.T, installed bool) (*fakeHost, *fakeTunService) {
	t.Helper()
	host := &fakeHost{}
	svc := &fakeTunService{handlers: make(chan func(*tunapi.Conn), 8)}
	oldDial, oldInstalled, oldSet, oldClear, oldPoll, oldRetry, oldVersion :=
		tunDial, tunInstalled, setTun, clearTun, tunPoll, tunRetryDelay, version.Current
	tunDial = svc.dial
	tunInstalled = func() (bool, error) { return installed, nil }
	setTun = func(index uint32, luid uint64) { host.note("bind") }
	clearTun = func() { host.note("unbind") }
	tunPoll = 10 * time.Millisecond
	tunRetryDelay = func(string, int) time.Duration { return 20 * time.Millisecond }
	version.Current = "2.0.0"
	t.Cleanup(func() {
		tunDial, tunInstalled, setTun, clearTun, tunPoll, tunRetryDelay, version.Current =
			oldDial, oldInstalled, oldSet, oldClear, oldPoll, oldRetry, oldVersion
	})
	return host, svc
}

func runSync(t *testing.T, host *fakeHost, bundled string) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		newTunSync(host, bundled).run(ctx)
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("tunSync did not stop")
		}
	}
}

func waitState(t *testing.T, host *fakeHost, state, code string) service.TunStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s := host.current()
		if s.State == state && s.Code == code {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("state %+v, want %s %s; steps: %s", s, state, code, host.steps())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func expect(t *testing.T, c *tunapi.Conn, typ string) tunapi.Message {
	t.Helper()
	m, err := c.Receive()
	if err != nil || m.Type != typ {
		t.Errorf("got %+v %v, want %s", m, err, typ)
	}
	return m
}

// healthyService answers a TUN session at version v, until closed; it
// reports on closed when the core closed it.
func healthyService(t *testing.T, v string, closed chan<- struct{}) func(*tunapi.Conn) {
	return func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello(v))
		session(t, c, closed)
	}
}

// session serves a TUN session after the hello.
func session(t *testing.T, c *tunapi.Conn, closed chan<- struct{}) {
	open := expect(t, c, tunapi.TypeOpen)
	if open.Proxy != "127.0.0.1:5555" || open.Username != "tun" || open.Password != "secret" {
		t.Errorf("open %+v", open)
	}
	c.Send(tunapi.Message{Type: tunapi.TypeOpened, Interface: &tunapi.Interface{Name: "Firefox VPN", Index: 7, LUID: 9}})
	expect(t, c, tunapi.TypeRoute)
	c.Send(tunapi.Message{Type: tunapi.TypeRouted})
	m, err := c.Receive()
	if err == nil && m.Type == tunapi.TypeClose {
		c.Send(tunapi.Message{Type: tunapi.TypeClosed})
		if closed != nil {
			close(closed)
		}
	}
}

// The TUN follows the mode: opened, the core bound outside it and
// reconnected before anything is routed, and closed before the core's
// connections may use the routes again.
func TestTheTUNFollowsTheModeInOrder(t *testing.T) {
	host, svc := setup(t, true)
	closed := make(chan struct{})
	svc.handlers <- healthyService(t, "2.0.0", closed)
	stop := runSync(t, host, "")
	defer stop()
	waitState(t, host, service.TunOff, "")
	host.wanted.Store(true)
	on := waitState(t, host, service.TunOn, "")
	if on.ServiceVersion != "2.0.0" {
		t.Fatalf("service version %q", on.ServiceVersion)
	}
	if got := host.steps(); got != "listen bind reconnect" {
		t.Fatalf("steps %q, want listen, bind, reconnect before the route", got)
	}
	host.wanted.Store(false)
	waitState(t, host, service.TunOff, "")
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the TUN was not closed")
	}
	if got := host.steps(); got != "listen bind reconnect unbind unlisten" {
		t.Fatalf("steps %q", got)
	}
}

// The service gets the programs that skip the VPN with the open, and again
// when they change while the TUN runs; a program whose call it refused
// reaches the daemon. A service of protocol 1 is sent no programs message.
func TestTheServiceGetsTheProgramsThatSkipTheVPN(t *testing.T) {
	host, svc := setup(t, true)
	host.setPrograms(config.TunProgram{Name: "steam.exe"})
	changed, closed := make(chan []tunapi.Program, 1), make(chan struct{})
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("2.0.0"))
		open := expect(t, c, tunapi.TypeOpen)
		if want := []tunapi.Program{{Name: "steam.exe"}}; !slices.Equal(open.Programs, want) {
			t.Errorf("opened with %+v, want %+v", open.Programs, want)
		}
		c.Send(tunapi.Message{Type: tunapi.TypeOpened, Interface: &tunapi.Interface{Name: "Firefox VPN", Index: 7, LUID: 9}})
		expect(t, c, tunapi.TypeRoute)
		c.Send(tunapi.Message{Type: tunapi.TypeRouted})
		c.Send(tunapi.Message{Type: tunapi.TypeUDPRefused, Program: "Discord.exe", Path: `C:\Apps\Discord.exe`})
		m := expect(t, c, tunapi.TypePrograms)
		c.Send(tunapi.Message{Type: tunapi.TypeProgramsSet})
		changed <- m.Programs
		if m, err := c.Receive(); err == nil && m.Type == tunapi.TypeClose {
			c.Send(tunapi.Message{Type: tunapi.TypeClosed})
			close(closed)
		}
	}
	host.wanted.Store(true)
	stop := runSync(t, host, "")
	defer stop()
	waitState(t, host, service.TunOn, "")
	host.setPrograms(config.TunProgram{Name: "steam.exe"}, config.TunProgram{Name: "Telegram.exe", UDPOnly: true})
	select {
	case got := <-changed:
		if want := []tunapi.Program{{Name: "steam.exe"}, {Name: "Telegram.exe", UDPOnly: true}}; !slices.Equal(got, want) {
			t.Errorf("changed to %+v, want %+v", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the changed programs were not sent")
	}
	if got := host.refusals(); !slices.Equal(got, []string{`Discord.exe C:\Apps\Discord.exe`}) {
		t.Errorf("refused UDP noted %v", got)
	}
	host.wanted.Store(false)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the TUN was not closed")
	}

	// Protocol 1: the open carries the programs (ignored there), and a
	// change sends nothing until the close.
	old := make(chan string, 1)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Message{Type: tunapi.TypeHello, Protocol: 1, MinProtocol: 1, Version: "2.0.0"})
		expect(t, c, tunapi.TypeOpen)
		c.Send(tunapi.Message{Type: tunapi.TypeOpened, Interface: &tunapi.Interface{Name: "Firefox VPN", Index: 7, LUID: 9}})
		expect(t, c, tunapi.TypeRoute)
		c.Send(tunapi.Message{Type: tunapi.TypeRouted})
		m, _ := c.Receive()
		old <- m.Type
		c.Send(tunapi.Message{Type: tunapi.TypeClosed})
	}
	host.wanted.Store(true)
	waitState(t, host, service.TunOn, "")
	host.setPrograms()
	time.Sleep(50 * time.Millisecond)
	host.wanted.Store(false)
	select {
	case got := <-old:
		if got != tunapi.TypeClose {
			t.Errorf("a service of protocol 1 got %q before the close", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the TUN was not closed")
	}
}

// Quitting the app closes the TUN.
func TestQuittingClosesTheTUN(t *testing.T) {
	host, svc := setup(t, true)
	closed := make(chan struct{})
	svc.handlers <- healthyService(t, "2.0.0", closed)
	host.wanted.Store(true)
	stop := runSync(t, host, "")
	waitState(t, host, service.TunOn, "")
	stop()
	select {
	case <-closed:
	default:
		t.Fatal("quitting left the TUN open")
	}
	if !strings.HasSuffix(host.steps(), "unbind unlisten") {
		t.Fatalf("steps %q", host.steps())
	}
}

func TestAMissingServiceIsReportedForTheAppToInstall(t *testing.T) {
	host, _ := setup(t, false)
	host.wanted.Store(true)
	defer runSync(t, host, "")()
	s := waitState(t, host, service.TunError, service.TunCodeMissing)
	if !strings.Contains(s.Message, "not installed") {
		t.Fatalf("message %q", s.Message)
	}
}

// A service another user of the computer installed refuses this one (a
// Unix socket's answer to hello): the app installs it for this user too.
func TestAServiceOfAnotherUserIsReportedForTheAppToInstall(t *testing.T) {
	host, svc := setup(t, true)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Errorf(tunapi.CodeNotAllowed, "the TUN service was installed for another user"))
	}
	host.wanted.Store(true)
	defer runSync(t, host, "")()
	s := waitState(t, host, service.TunError, service.TunCodeMissing)
	if !strings.Contains(s.Message, "another user") {
		t.Fatalf("message %q", s.Message)
	}
	// Installed for this user, it answers.
	svc.handlers <- healthyService(t, "2.0.0", nil)
	waitState(t, host, service.TunOn, "")
}

func TestAnInstalledServiceThatDoesNotAnswerIsRetried(t *testing.T) {
	host, svc := setup(t, true)
	host.wanted.Store(true)
	defer runSync(t, host, "")()
	waitState(t, host, service.TunError, service.TunCodeNotRunning)
	// It comes up: the next try opens the TUN.
	svc.handlers <- healthyService(t, "2.0.0", nil)
	waitState(t, host, service.TunOn, "")
}

// A TUN that stops by itself (the adapter disabled) unbinds the core at
// once and starts again.
func TestALostTUNStartsAgain(t *testing.T) {
	host, svc := setup(t, true)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("2.0.0"))
		expect(t, c, tunapi.TypeOpen)
		c.Send(tunapi.Message{Type: tunapi.TypeOpened, Interface: &tunapi.Interface{Index: 7, LUID: 9}})
		expect(t, c, tunapi.TypeRoute)
		c.Send(tunapi.Message{Type: tunapi.TypeRouted})
		c.Send(tunapi.Message{Type: tunapi.TypeFailed, Code: tunapi.CodeAdapterLost, Message: "the TUN adapter was disabled or removed"})
	}
	host.wanted.Store(true)
	defer runSync(t, host, "")()
	s := waitState(t, host, service.TunError, service.TunCodeLost)
	if !strings.Contains(s.Message, "disabled") {
		t.Fatalf("message %q", s.Message)
	}
	if got := host.steps(); got != "listen bind reconnect unbind unlisten" {
		t.Fatalf("steps %q", got)
	}
	svc.handlers <- healthyService(t, "2.0.0", nil)
	waitState(t, host, service.TunOn, "")
}

// An older service is updated from the copy beside the core, and the TUN
// opens with the new one.
func TestAnOlderServiceIsUpdatedFirst(t *testing.T) {
	host, svc := setup(t, true)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("1.0.0"))
		if m := expect(t, c, tunapi.TypeUpdate); m.Path != `C:\App\core\firefox-vpn-tun.exe` {
			t.Errorf("update from %q", m.Path)
		}
		c.Send(tunapi.Message{Type: tunapi.TypeUpdating})
	}
	svc.handlers <- func(*tunapi.Conn) {} // the wait for the restart
	svc.handlers <- healthyService(t, "2.0.0", nil)
	host.wanted.Store(true)
	defer runSync(t, host, `C:\App\core\firefox-vpn-tun.exe`)()
	if s := waitState(t, host, service.TunOn, ""); s.ServiceVersion != "2.0.0" {
		t.Fatalf("on with %q", s.ServiceVersion)
	}
}

// After an update of the app, an older service updates itself at once,
// while the TUN mode is not used.
func TestAfterTheAppsUpdateAnOlderServiceUpdatesAtOnce(t *testing.T) {
	host, svc := setup(t, true)
	updated := make(chan string, 1)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("1.0.0"))
		updated <- expect(t, c, tunapi.TypeUpdate).Path
		c.Send(tunapi.Message{Type: tunapi.TypeUpdating})
	}
	svc.handlers <- func(*tunapi.Conn) {} // the wait for the restart
	left := make(chan struct{})
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("2.0.0"))
		if _, err := c.Receive(); err == nil {
			t.Error("the core asked more of the service")
		}
		close(left)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sync := newTunSync(host, `C:\App\core\firefox-vpn-tun.exe`)
	sync.refresh = true
	go sync.run(ctx)
	select {
	case path := <-updated:
		if path != `C:\App\core\firefox-vpn-tun.exe` {
			t.Fatalf("update from %q", path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no update")
	}
	select {
	case <-left:
	case <-time.After(5 * time.Second):
		t.Fatal("the core kept the service")
	}
	if s := host.current(); s.State != service.TunOff {
		t.Fatalf("state %+v", s)
	}
}

// An older service that speaks the protocol but refuses the update (not
// signed by its key) is used as it is.
func TestACompatibleServiceThatRefusesTheUpdateIsStillUsed(t *testing.T) {
	host, svc := setup(t, true)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("1.0.0"))
		expect(t, c, tunapi.TypeUpdate)
		c.Send(tunapi.Errorf(tunapi.CodeUpdateRefused, "not signed"))
		session(t, c, nil)
	}
	host.wanted.Store(true)
	defer runSync(t, host, `C:\App\core\firefox-vpn-tun.exe`)()
	waitState(t, host, service.TunOn, "")
}

func TestAServiceOfAnotherProtocol(t *testing.T) {
	for _, c := range []struct {
		name          string
		protocol, min int
		code          string
	}{
		{"older, not updatable", 0, 0, service.TunCodeOutdated},
		{"newer than the app", tunapi.Protocol + 5, tunapi.Protocol + 3, service.TunCodeAppOutdated},
	} {
		t.Run(c.name, func(t *testing.T) {
			host, svc := setup(t, true)
			svc.handlers <- func(conn *tunapi.Conn) {
				expect(t, conn, tunapi.TypeHello)
				reply := tunapi.Errorf(tunapi.CodeProtocol, "no common protocol")
				reply.Protocol, reply.MinProtocol, reply.Version = c.protocol, c.min, "0.9.0"
				conn.Send(reply)
				if m, err := conn.Receive(); err == nil && m.Type == tunapi.TypeUpdate {
					conn.Send(tunapi.Errorf(tunapi.CodeUpdateRefused, "not signed"))
				}
			}
			host.wanted.Store(true)
			defer runSync(t, host, `C:\App\core\firefox-vpn-tun.exe`)()
			waitState(t, host, service.TunError, c.code)
		})
	}
}

func TestAnotherCopysTUNMakesItBusy(t *testing.T) {
	host, svc := setup(t, true)
	svc.handlers <- func(c *tunapi.Conn) {
		expect(t, c, tunapi.TypeHello)
		c.Send(tunapi.Hello("2.0.0"))
		expect(t, c, tunapi.TypeOpen)
		c.Send(tunapi.Errorf(tunapi.CodeBusy, "another program's TUN session runs"))
	}
	host.wanted.Store(true)
	defer runSync(t, host, "")()
	waitState(t, host, service.TunError, service.TunCodeBusy)
	if got := host.steps(); got != "listen unlisten" {
		t.Fatalf("steps %q", got)
	}
}
