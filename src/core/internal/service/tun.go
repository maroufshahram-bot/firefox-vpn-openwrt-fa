package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/config"
	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
)

// The desktop TUN mode (system_proxy.mode "tun"): the TUN service
// (cmd/firefox-vpn-tun) sends every TCP connection of the computer to a
// SOCKS5 listener of its own here, on loopback with a login made for this
// process, so that another program holding proxy.listen never receives
// them, a taken proxy port does not stop the TUN, and the user's proxy
// login does not apply. The running engine serves them like its other
// clients, with the VPN's rule (TunnelAllows). The process hosting the
// service (the desktop app's core, internal/desktop) runs the TUN session
// and reports its state here.

// TunStatus is the TUN mode's state in /api/status.
type TunStatus struct {
	// State is TunOff, TunStarting, TunOn, or TunError.
	State string `json:"state"`
	// Code says why the TUN is not on (a TunCode constant), so that apps
	// word it themselves.
	Code string `json:"code,omitempty"`
	// Message is the reason in English.
	Message string `json:"message,omitempty"`
	// ServiceVersion is the TUN service's version, once it answered.
	ServiceVersion string `json:"service_version,omitempty"`
	// Installed: the service is installed (the app installs it before it
	// turns the TUN mode on).
	Installed bool `json:"installed"`
	// ProgramsSupported: programs can skip the VPN here (tun.programs; the
	// TUN service finds each connection's program on Windows and Linux).
	ProgramsSupported bool `json:"programs_supported"`
	// RefusedUDP is the last program whose UDP that looks like a call or a
	// game the running TUN refused (Mozilla carries no UDP), while it is not
	// in tun.programs: the app offers to send it directly. RefusedUDPPath
	// is its executable.
	RefusedUDP     string `json:"refused_udp,omitempty"`
	RefusedUDPPath string `json:"refused_udp_path,omitempty"`
}

// tunProgramsSupported: TunStatus.ProgramsSupported.
var tunProgramsSupported = runtime.GOOS == "windows" || runtime.GOOS == "linux"

// RunningProgram is a program running on this computer: its executable's
// file name, which tun.programs takes, and its path, which tells two of the
// same name apart.
type RunningProgram struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// TUN states.
const (
	TunOff      = "off"
	TunStarting = "starting"
	TunOn       = "on"
	TunError    = "error"
)

// TUN codes.
const (
	// TunCodeMissing: the service is not installed, or not for this user;
	// the app installs it.
	TunCodeMissing = "missing"
	// TunCodeNotRunning: it is installed but does not answer; retried.
	TunCodeNotRunning = "not_running"
	// TunCodeOutdated: it is older than the app and could not update
	// itself; installing it again replaces it.
	TunCodeOutdated = "outdated"
	// TunCodeAppOutdated: it is newer than the app and speaks no protocol
	// the app does; the app must be updated.
	TunCodeAppOutdated = "app_outdated"
	// TunCodeBusy: another copy of the program's TUN runs.
	TunCodeBusy = "busy"
	// TunCodeFailed: the adapter, its routes, or the DNS filters failed;
	// the message says why. Retried.
	TunCodeFailed = "failed"
	// TunCodeLost: the TUN stopped by itself (the adapter was disabled, the
	// service stopped); retried.
	TunCodeLost = "lost"
)

// tunMaxConnections bounds the TUN listener's connections: every app's.
const tunMaxConnections = 4096

// tunCredentials is the login of the TUN listener, made once per process.
func (d *Daemon) tunCredentials() (user, password string) {
	d.tun.mu.Lock()
	defer d.tun.mu.Unlock()
	if d.tun.password == "" {
		b := make([]byte, 24)
		_, _ = rand.Read(b)
		d.tun.user, d.tun.password = "tun", base64.RawURLEncoding.EncodeToString(b)
	}
	return d.tun.user, d.tun.password
}

// OpenTunListener opens the TUN service's listener and returns its address
// and login; CloseTunListener closes it.
func (d *Daemon) OpenTunListener() (addr, user, password string, err error) {
	user, password = d.tunCredentials()
	d.tun.mu.Lock()
	defer d.tun.mu.Unlock()
	if d.tun.listener != nil {
		return d.tun.listener.Addr().String(), user, password, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", "", err
	}
	d.tun.listener = ln
	go d.serveTun(ln)
	logx.Info("TUN listener opened listen=%s", ln.Addr())
	return ln.Addr().String(), user, password, nil
}

// CloseTunListener closes the TUN listener; its connections end with the
// TUN.
func (d *Daemon) CloseTunListener() {
	d.tun.mu.Lock()
	ln := d.tun.listener
	d.tun.listener = nil
	d.tun.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
}

// serveTun hands each connection to the running engine; without one (the
// proxy restarting, waiting, or failed) it is closed at once, so the app
// is told.
func (d *Daemon) serveTun(ln net.Listener) {
	var backoff time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			backoff = min(max(2*backoff, 5*time.Millisecond), time.Second)
			time.Sleep(backoff)
			continue
		}
		backoff = 0
		engine := d.currentEngine()
		if engine == nil || engine.tunSOCKS == nil {
			_ = conn.Close()
			continue
		}
		go engine.tunSOCKS.ServeConn(conn)
	}
}

// TunWanted reports whether the TUN should run: the TUN mode is chosen,
// the proxy is on, and auto-connect does not wait for its first test.
func (d *Daemon) TunWanted() bool {
	cfg := d.config()
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.TunSupported && cfg.SystemProxy.Mode == config.SystemProxyTun && !d.off && d.autoConnect == nil
}

// SetTunStatus records the TUN's state; the process running the session
// reports it.
func (d *Daemon) SetTunStatus(s TunStatus) {
	d.tun.mu.Lock()
	changed := d.tun.status == nil || *d.tun.status != s
	d.tun.status = &s
	if s.State != TunOn {
		d.tun.refusedUDP, d.tun.refusedUDPPath = "", ""
	}
	d.tun.mu.Unlock()
	if changed {
		logx.Info("TUN state=%s code=%s message=%q", s.State, s.Code, s.Message)
	}
}

// TunPrograms is tun.programs, which the TUN sends directly.
func (d *Daemon) TunPrograms() []config.TunProgram {
	return slices.Clone(d.config().Tun.Programs)
}

// NoteRefusedUDP records the program (its file name, and its executable)
// whose UDP (a call or a game) the TUN refused; the TUN service tells once
// per program while it runs.
func (d *Daemon) NoteRefusedUDP(program, path string) {
	if program == "" {
		return
	}
	d.tun.mu.Lock()
	d.tun.refusedUDP, d.tun.refusedUDPPath = program, path
	d.tun.mu.Unlock()
	logx.Info("TUN: UDP of program=%s was refused (a call or a game; Mozilla carries no UDP)", program)
}

// tunStatus is the TUN's state for cfg's programs; StatusView calls it
// holding d.mu, so it must not take it again.
func (d *Daemon) tunStatus(cfg config.Config) *TunStatus {
	if !d.TunSupported {
		return nil
	}
	programs := cfg.Tun.Programs
	d.tun.mu.Lock()
	s := TunStatus{State: TunOff}
	if d.tun.status != nil {
		s = *d.tun.status
	}
	refused, refusedPath := d.tun.refusedUDP, d.tun.refusedUDPPath
	d.tun.mu.Unlock()
	s.Installed = d.TunInstalled != nil && d.TunInstalled()
	s.ProgramsSupported = tunProgramsSupported
	// Once it skips the VPN, by name or by a pattern, there is nothing left
	// to offer.
	if s.State == TunOn && !slices.ContainsFunc(programs, func(p config.TunProgram) bool {
		return matchProgram(p.Name, refused)
	}) {
		s.RefusedUDP, s.RefusedUDPPath = refused, refusedPath
	}
	return &s
}

// matchProgram is tunapi.MatchProgram, which this package does not import
// (the phone's core has no TUN service): a file name without case, or a
// pattern with * and ?.
func matchProgram(pattern, name string) bool {
	pattern, name = strings.ToLower(pattern), strings.ToLower(name)
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == name
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// ReconnectUpstream opens new upstream sessions: the TUN is starting, and
// the sessions opened before it are not bound outside its adapter. An
// engine asleep connects bound when it wakes.
func (d *Daemon) ReconnectUpstream() {
	engine := d.currentEngine()
	if engine == nil {
		return
	}
	controller := engine.currentController()
	if controller == nil {
		return
	}
	if err := controller.Reconnect(); err != nil {
		logx.Debug("reconnecting to Mozilla for the TUN failed err=%s", logx.Err(err))
	}
}
