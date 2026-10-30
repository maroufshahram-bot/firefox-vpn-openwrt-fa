package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/config"
	"github.com/armanlife/firefox-vpn-client/core/internal/egress"
	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/service"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// The TUN mode in the app's core process. While the daemon wants a TUN
// (Daemon.TunWanted), tunSync holds a session with the TUN service
// (cmd/firefox-vpn-tun, protocol internal/tunapi): it opens the adapter,
// binds the core's own connections outside it (egress), routes everything
// into it, and closes it when the mode changes, the proxy goes off, or the
// app quits. However this process ends, its pipe connection ends with it,
// and the service then takes the TUN down by itself.

// tunHost is the part of the daemon tunSync uses; tests fake it.
type tunHost interface {
	TunWanted() bool
	OpenTunListener() (addr, user, password string, err error)
	CloseTunListener()
	SetTunStatus(service.TunStatus)
	ReconnectUpstream()
	TunPrograms() []config.TunProgram
	NoteRefusedUDP(program, path string)
}

// Replaced in tests.
var (
	tunDial      = tunapi.Dial
	tunInstalled = tunapi.Installed
	setTun       = egress.SetTun
	clearTun     = egress.ClearTun
	tunPoll      = time.Second
	// tunRestartWait bounds the wait for the service to answer again after
	// it updated itself.
	tunRestartWait = 30 * time.Second
)

// tunReplyTimeout bounds a reply of the service; creating the adapter can
// take a few seconds.
const tunReplyTimeout = 30 * time.Second

type tunSync struct {
	host tunHost
	// bundled is the service executable beside the core, which an older
	// service is updated from.
	bundled string
	conn    *tunapi.Conn
	// events are the service's messages while the TUN runs; closed when
	// the connection ends.
	events   chan tunapi.Message
	retryAt  time.Time
	failures int
	// updated holds the service versions an update was asked of, so a
	// refused or ineffective update is not asked again.
	updated map[string]bool
	// refresh: an installed service older than this core is asked to
	// update itself at the start, even while the TUN mode is not used (the
	// app was just updated).
	refresh bool
	// protocol is the session's protocol version; programs are the
	// programs that skip the VPN as the service last got them.
	protocol int
	programs []tunapi.Program
}

func newTunSync(host tunHost, bundled string) *tunSync {
	return &tunSync{host: host, bundled: bundled, updated: map[string]bool{}}
}

// tunAvailable reports whether this process can run the TUN mode: on
// Windows, Linux, and macOS, with the service installed or beside the core
// to install, and with the core's own connections kept outside the TUN
// (egress.Supported: an older Linux does not let them be).
func tunAvailable() (bundled string, ok bool) {
	switch runtime.GOOS {
	case "windows", "linux", "darwin":
	default:
		return "", false
	}
	if err := egress.Supported(); err != nil {
		logx.Info("app: no TUN mode here: %s", logx.Err(err))
		return "", false
	}
	if exe, err := os.Executable(); err == nil {
		bundled = filepath.Join(filepath.Dir(exe), tunapi.ExeName)
		if _, err := os.Stat(bundled); err != nil {
			bundled = ""
		}
	}
	installed, _ := tunInstalled()
	return bundled, bundled != "" || installed
}

// refreshService asks an installed service that is older than this core
// to update itself from the copy beside the core (connect does), then
// leaves it.
func (t *tunSync) refreshService(ctx context.Context) {
	if installed, _ := tunInstalled(); !installed || t.bundled == "" {
		return
	}
	conn, peer, err := t.connect(ctx)
	if err != nil {
		logx.Warn("the TUN service did not update after the app's update service=%s err=%s", peer.Version, logx.Err(err))
		return
	}
	conn.Close()
}

// run follows the daemon's wish until ctx ends, then closes the TUN.
func (t *tunSync) run(ctx context.Context) {
	tick := time.NewTicker(tunPoll)
	defer tick.Stop()
	t.host.SetTunStatus(service.TunStatus{State: service.TunOff})
	if t.refresh {
		t.refreshService(ctx)
	}
	for {
		wanted := t.host.TunWanted()
		switch {
		case wanted && t.conn == nil && !time.Now().Before(t.retryAt):
			t.start(ctx)
		case !wanted && t.conn != nil:
			t.stop()
		case !wanted:
			t.failures, t.retryAt = 0, time.Time{}
			t.host.SetTunStatus(service.TunStatus{State: service.TunOff})
		}
		t.syncPrograms()
		var events <-chan tunapi.Message
		if t.conn != nil {
			events = t.events
		}
		select {
		case <-ctx.Done():
			if t.conn != nil {
				t.stop()
			}
			return
		case m, ok := <-events:
			switch {
			case !ok || m.Type == tunapi.TypeFailed:
				t.lost(m, ok)
			case m.Type == tunapi.TypeUDPRefused:
				t.host.NoteRefusedUDP(m.Program, m.Path)
			case m.Type == tunapi.TypeError:
				logx.Warn("the TUN service refused a request: %s", m.Message)
			}
		case <-tick.C:
		}
	}
}

// tunPrograms are the programs that skip the VPN, as the service takes
// them.
func tunPrograms(programs []config.TunProgram) []tunapi.Program {
	out := make([]tunapi.Program, 0, len(programs))
	for _, p := range programs {
		out = append(out, tunapi.Program{Name: p.Name, UDPOnly: p.UDPOnly})
	}
	return out
}

// syncPrograms gives a running TUN the programs that skip the VPN again
// when they changed (protocol 2; the reply comes with the events).
func (t *tunSync) syncPrograms() {
	if t.conn == nil || t.protocol < 2 {
		return
	}
	programs := tunPrograms(t.host.TunPrograms())
	if slices.Equal(programs, t.programs) {
		return
	}
	// A failed send ends the connection, and the events say so.
	if t.conn.Send(tunapi.Message{Type: tunapi.TypePrograms, Programs: programs}) == nil {
		t.programs = programs
		logx.Info("TUN: programs that skip the VPN updated count=%d", len(programs))
	}
}

// tunError is why a start failed, with its status code.
type tunError struct {
	code string
	err  error
}

func (e *tunError) Error() string { return e.err.Error() }
func (e *tunError) Unwrap() error { return e.err }

func failed(code string, format string, args ...any) error {
	return &tunError{code: code, err: fmt.Errorf(format, args...)}
}

// start opens and routes the TUN.
func (t *tunSync) start(ctx context.Context) {
	t.host.SetTunStatus(service.TunStatus{State: service.TunStarting})
	conn, peer, err := t.connect(ctx)
	if err != nil {
		t.fail(err, "")
		return
	}
	serviceVersion := peer.Version
	addr, user, password, err := t.host.OpenTunListener()
	if err != nil {
		conn.Close()
		t.fail(failed(service.TunCodeFailed, "opening the TUN listener: %w", err), serviceVersion)
		return
	}
	// A service of protocol 1 ignores the programs.
	programs := tunPrograms(t.host.TunPrograms())
	reply, err := t.ask(conn, tunapi.Message{Type: tunapi.TypeOpen, Proxy: addr, Username: user, Password: password, Programs: programs})
	if err == nil && (reply.Type != tunapi.TypeOpened || reply.Interface == nil) {
		err = fmt.Errorf("unexpected reply %q", reply.Type)
	}
	if err != nil {
		conn.Close()
		t.host.CloseTunListener()
		code := service.TunCodeFailed
		var e *tunapi.Error
		if errors.As(err, &e) && e.Code == tunapi.CodeBusy {
			code = service.TunCodeBusy
		}
		t.fail(&tunError{code: code, err: err}, serviceVersion)
		return
	}
	iface := reply.Interface
	// The core's connections leave outside the adapter before anything is
	// routed into it: the sessions to Mozilla opened before it are
	// replaced by bound ones.
	setTun(iface.Index, iface.LUID)
	t.host.ReconnectUpstream()
	if reply, err = t.ask(conn, tunapi.Message{Type: tunapi.TypeRoute}); err == nil && reply.Type != tunapi.TypeRouted {
		err = fmt.Errorf("unexpected reply %q", reply.Type)
	}
	if err != nil {
		// The service took the adapter down with the failed route.
		conn.Close()
		clearTun()
		t.host.CloseTunListener()
		t.fail(failed(service.TunCodeFailed, "%w", err), serviceVersion)
		return
	}
	events := make(chan tunapi.Message, 4)
	go func() {
		defer close(events)
		for {
			m, err := conn.Receive()
			if err != nil {
				return
			}
			events <- m
		}
	}()
	t.conn, t.events = conn, events
	t.protocol, t.programs = tunapi.Common(peer.Protocol, peer.MinProtocol), programs
	t.failures, t.retryAt = 0, time.Time{}
	logx.Info("TUN on adapter=%q index=%d", iface.Name, iface.Index)
	t.host.SetTunStatus(service.TunStatus{State: service.TunOn, ServiceVersion: serviceVersion})
}

// connect says hello to the service, updating it first when it is older
// than this core, and returns the connection and the service's hello (its
// version and protocols; the version alone after a failure).
func (t *tunSync) connect(ctx context.Context) (*tunapi.Conn, tunapi.Message, error) {
	for attempt := 0; ; attempt++ {
		conn, hello, err := t.dialHello(ctx)
		if err != nil {
			return nil, tunapi.Message{}, err
		}
		peer := tunapi.Message{Version: hello.Version}
		incompatible := hello.Type == tunapi.TypeError
		if incompatible && hello.MinProtocol > tunapi.Protocol {
			conn.Close()
			return nil, peer, failed(service.TunCodeAppOutdated, "the TUN service %s is newer than this app %s", hello.Version, version.Current)
		}
		older := incompatible || version.Newer(version.Current, hello.Version)
		if !older || attempt > 0 || t.bundled == "" || t.updated[hello.Version] {
			if incompatible {
				conn.Close()
				return nil, peer, failed(service.TunCodeOutdated, "the TUN service %s is older than this app %s", hello.Version, version.Current)
			}
			return conn, hello, nil
		}
		t.updated[hello.Version] = true
		logx.Info("updating the TUN service from=%s to=%s", hello.Version, version.Current)
		reply, err := t.ask(conn, tunapi.Message{Type: tunapi.TypeUpdate, Path: t.bundled})
		if err == nil && reply.Type != tunapi.TypeUpdating {
			err = fmt.Errorf("unexpected reply %q", reply.Type)
		}
		if err != nil {
			logx.Warn("the TUN service did not update itself err=%s", logx.Err(err))
			if incompatible {
				conn.Close()
				return nil, peer, failed(service.TunCodeOutdated, "the TUN service %s is older than this app and did not update: %w", hello.Version, err)
			}
			// It still speaks this core's protocol.
			return conn, hello, nil
		}
		conn.Close()
		if err := t.waitForService(ctx); err != nil {
			return nil, tunapi.Message{}, err
		}
	}
}

// dialHello connects and says hello; the reply is the service's hello, or
// an error naming its protocol range.
func (t *tunSync) dialHello(ctx context.Context) (*tunapi.Conn, tunapi.Message, error) {
	dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	raw, err := tunDial(dctx)
	cancel()
	if err != nil {
		switch {
		case errors.Is(err, tunapi.ErrNotAllowed):
			return nil, tunapi.Message{}, failed(service.TunCodeMissing, "%w", err)
		case errors.Is(err, tunapi.ErrNoService):
			if installed, ierr := tunInstalled(); ierr == nil && !installed {
				return nil, tunapi.Message{}, failed(service.TunCodeMissing, "the TUN service is not installed")
			}
			return nil, tunapi.Message{}, failed(service.TunCodeNotRunning, "%w", err)
		}
		return nil, tunapi.Message{}, failed(service.TunCodeFailed, "connecting to the TUN service: %w", err)
	}
	conn := tunapi.NewConn(raw)
	reply, err := t.ask(conn, tunapi.Hello(version.Current))
	var e *tunapi.Error
	if errors.As(err, &e) && e.Code == tunapi.CodeProtocol {
		return conn, tunapi.Message{Type: tunapi.TypeError, Code: e.Code, Protocol: e.Protocol, MinProtocol: e.MinProtocol, Version: e.Version}, nil
	}
	if errors.As(err, &e) && e.Code == tunapi.CodeNotAllowed {
		// Installed by another user of this computer (a Unix socket): the
		// app installs it for this one too.
		conn.Close()
		return nil, tunapi.Message{}, failed(service.TunCodeMissing, "%w", tunapi.ErrNotAllowed)
	}
	if err == nil && reply.Type != tunapi.TypeHello {
		err = fmt.Errorf("unexpected reply %q", reply.Type)
	}
	if err != nil {
		conn.Close()
		return nil, tunapi.Message{}, failed(service.TunCodeFailed, "talking to the TUN service: %w", err)
	}
	return conn, reply, nil
}

// waitForService waits for an updated service to answer.
func (t *tunSync) waitForService(ctx context.Context) error {
	deadline := time.Now().Add(tunRestartWait)
	for {
		dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		raw, err := tunDial(dctx)
		cancel()
		if err == nil {
			raw.Close()
			return nil
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return failed(service.TunCodeNotRunning, "the TUN service did not come back after updating: %w", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ask sends m and returns the reply, as an error when it is one.
func (t *tunSync) ask(conn *tunapi.Conn, m tunapi.Message) (tunapi.Message, error) {
	_ = conn.Raw().SetDeadline(time.Now().Add(tunReplyTimeout))
	defer conn.Raw().SetDeadline(time.Time{})
	if err := conn.Send(m); err != nil {
		return tunapi.Message{}, err
	}
	reply, err := conn.Receive()
	if err != nil {
		return tunapi.Message{}, err
	}
	if err := tunapi.AsError(reply); err != nil {
		return reply, err
	}
	return reply, nil
}

// stop closes the TUN, then lets the core's connections use the routes
// again: the service removed them first.
func (t *tunSync) stop() {
	conn, events := t.conn, t.events
	t.conn, t.events = nil, nil
	_ = conn.Raw().SetWriteDeadline(time.Now().Add(5 * time.Second))
	if conn.Send(tunapi.Message{Type: tunapi.TypeClose}) == nil {
		deadline := time.After(10 * time.Second)
	wait:
		for {
			select {
			case m, ok := <-events:
				if !ok || m.Type == tunapi.TypeClosed {
					break wait
				}
			case <-deadline:
				break wait
			}
		}
	}
	conn.Close()
	for range events {
	}
	clearTun()
	t.host.CloseTunListener()
	logx.Info("TUN off")
	t.host.SetTunStatus(service.TunStatus{State: service.TunOff})
}

// lost ends a TUN that stopped by itself (m, when ok, says why) and
// schedules another start.
func (t *tunSync) lost(m tunapi.Message, ok bool) {
	conn := t.conn
	t.conn, t.events = nil, nil
	conn.Close()
	clearTun()
	t.host.CloseTunListener()
	message := "the TUN service stopped"
	if ok && m.Message != "" {
		message = m.Message
	}
	logx.Warn("the TUN stopped by itself: %s", message)
	t.failures++
	t.retryAt = time.Now().Add(tunRetryDelay(service.TunCodeLost, t.failures))
	t.host.SetTunStatus(service.TunStatus{State: service.TunError, Code: service.TunCodeLost, Message: message})
}

// fail reports a start that failed and when the next one is.
func (t *tunSync) fail(err error, serviceVersion string) {
	code := service.TunCodeFailed
	var te *tunError
	if errors.As(err, &te) {
		code = te.code
	}
	t.failures++
	t.retryAt = time.Now().Add(tunRetryDelay(code, t.failures))
	if code != service.TunCodeMissing || t.failures == 1 {
		logx.Warn("starting the TUN failed code=%s err=%s", code, logx.Err(err))
	}
	t.host.SetTunStatus(service.TunStatus{State: service.TunError, Code: code, Message: err.Error(), ServiceVersion: serviceVersion})
}

// tunRetryDelay is the wait before the next start after failures: short
// while the app installs the service, longer for what retrying rarely
// fixes. Tests replace it.
var tunRetryDelay = func(code string, failures int) time.Duration {
	switch code {
	case service.TunCodeMissing:
		return 2 * time.Second
	case service.TunCodeAppOutdated, service.TunCodeOutdated:
		return time.Minute
	case service.TunCodeBusy, service.TunCodeNotRunning:
		return 5 * time.Second
	}
	return min(5*time.Second<<min(failures-1, 4), time.Minute)
}
