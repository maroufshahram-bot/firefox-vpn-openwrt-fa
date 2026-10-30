package service

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/controlplane"
	"github.com/armanlife/firefox-vpn-client/core/internal/update"
)

// The desktop app's updates (internal/update): the app finds a newer
// release and asks its core to download it (POST /api/update/download) and,
// once downloaded, to install it (POST /api/update/install); the status
// carries the progress (StatusView.Update). Only the program on this
// computer may ask, with the local token.

// UpdateTransports are the ways an update downloads, in order: through the
// proxy while it is on, as the proxy's clients connect (waking it, and
// routed), since GitHub may be blocked; then directly.
func (d *Daemon) UpdateTransports() []http.RoundTripper {
	var out []http.RoundTripper
	if engine := d.currentEngine(); engine != nil {
		out = append(out, &http.Transport{
			Proxy: nil,
			DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
				return engine.routed.OpenTunnel(address)
			},
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			TLSHandshakeTimeout: 30 * time.Second,
			ForceAttemptHTTP2:   true,
		})
	}
	return append(out, controlplane.Transport())
}

// handleUpdateDownload starts downloading {"version": "1.2.3"}.
func (s *dashboardServer) handleUpdateDownload(w http.ResponseWriter, r *http.Request) {
	m := s.updater(w, r)
	if m == nil {
		return
	}
	var in struct {
		Version string `json:"version"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := m.Download(in.Version); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m.Status())
}

// handleUpdateInstall installs the downloaded {"version": "1.2.3"}; "pid"
// is the app's process, which quits once this answers, and "args" are its
// arguments for the next start. The proxy comes back on after the update
// if it is on now.
func (s *dashboardServer) handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	m := s.updater(w, r)
	if m == nil {
		return
	}
	var in struct {
		Version string   `json:"version"`
		PID     int      `json:"pid"`
		Args    []string `json:"args"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := m.Install(update.InstallRequest{Version: in.Version, PID: in.PID, Args: in.Args, On: s.d.IsOn()}); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m.Status())
}

// updater returns the daemon's updater, or answers why there is none.
func (s *dashboardServer) updater(w http.ResponseWriter, r *http.Request) *update.Manager {
	if !s.d.hasLocalToken(r) {
		writeError(w, http.StatusForbidden, "only the app on this computer can update it")
		return nil
	}
	if s.d.Updater == nil {
		writeError(w, http.StatusConflict, "this copy does not update itself; download the new version instead")
		return nil
	}
	return s.d.Updater
}
