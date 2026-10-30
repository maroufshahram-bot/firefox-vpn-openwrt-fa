package service

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/config"
)

// status is the TUN's state as GET /api/status reports it.
func status(t *testing.T, d *Daemon) *TunStatus {
	t.Helper()
	done := make(chan StatusView, 1)
	go func() {
		view, err := d.StatusView()
		if err != nil {
			t.Error(err)
		}
		done <- view
	}()
	select {
	case view := <-done:
		return view.Tun
	case <-time.After(5 * time.Second):
		t.Fatal("the status did not come: a lock is taken twice")
		return nil
	}
}

// The running programs come only from the desktop app's core, which lists
// them; elsewhere the API has none to show.
func TestTheRunningProgramsComeFromTheDesktopCore(t *testing.T) {
	d := newTestDaemon(t)
	c := newDashboardClient(t, d)
	if rec := c.do("GET", "/api/tun/programs", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("without the TUN mode: %d %s", rec.Code, rec.Body)
	}
	d.TunSupported = true
	d.RunningPrograms = func() ([]RunningProgram, error) {
		return []RunningProgram{{Name: "steam.exe", Path: `C:\Games\Steam\steam.exe`}}, nil
	}
	rec := c.do("GET", "/api/tun/programs", nil)
	var got struct{ Programs []RunningProgram }
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil ||
		!slices.Equal(got.Programs, []RunningProgram{{Name: "steam.exe", Path: `C:\Games\Steam\steam.exe`}}) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	d.RunningPrograms = func() ([]RunningProgram, error) { return nil, nil }
	if rec := c.do("GET", "/api/tun/programs", nil); rec.Body.String() != "{\"programs\":[]}\n" {
		t.Fatalf("none running: %s", rec.Body)
	}
}

// The program whose call the TUN refused shows while the TUN is on and the
// program does not skip the VPN: once the settings send it directly (by its
// name, or by a pattern) it is gone, and the TUN going off forgets it.
func TestTheRefusedProgramShowsWhileTheTUNRuns(t *testing.T) {
	d := newTestDaemon(t)
	d.TunSupported = true
	d.SetTunStatus(TunStatus{State: TunOn})
	if s := status(t, d); s.RefusedUDP != "" || s.ProgramsSupported != tunProgramsSupported {
		t.Fatalf("status %+v", s)
	}
	d.NoteRefusedUDP("Discord.exe", `C:\Apps\Discord.exe`)
	if s := status(t, d); s.RefusedUDP != "Discord.exe" || s.RefusedUDPPath != `C:\Apps\Discord.exe` {
		t.Fatalf("refused %q at %q, want Discord.exe", s.RefusedUDP, s.RefusedUDPPath)
	}
	cfg := d.config()
	cfg.Tun.Programs = []config.TunProgram{{Name: `C:\Apps\discord.exe`, UDPOnly: true}}
	if _, err := d.updateSettings(cfg, nil); err != nil {
		t.Fatal(err)
	}
	// The folder given with the name is kept as its path.
	if want := []config.TunProgram{{Name: "discord.exe", UDPOnly: true, Path: `C:\Apps\discord.exe`}}; !slices.Equal(d.TunPrograms(), want) {
		t.Fatalf("programs %+v, want %+v", d.TunPrograms(), want)
	}
	if s := status(t, d); s.RefusedUDP != "" {
		t.Fatalf("a program that skips the VPN is still offered: %q", s.RefusedUDP)
	}
	d.NoteRefusedUDP("SteamWebHelper.exe", "")
	if s := status(t, d); s.RefusedUDP != "SteamWebHelper.exe" {
		t.Fatalf("refused %q, want SteamWebHelper.exe", s.RefusedUDP)
	}
	cfg.Tun.Programs = append(cfg.Tun.Programs, config.TunProgram{Name: "steam*", UDPOnly: true})
	if _, err := d.updateSettings(cfg, nil); err != nil {
		t.Fatal(err)
	}
	if s := status(t, d); s.RefusedUDP != "" {
		t.Fatalf("a program a pattern sends directly is still offered: %q", s.RefusedUDP)
	}
	d.NoteRefusedUDP("Game.exe", "")
	d.SetTunStatus(TunStatus{State: TunOff})
	d.SetTunStatus(TunStatus{State: TunOn})
	if s := status(t, d); s.RefusedUDP != "" {
		t.Fatalf("the TUN off kept %q", s.RefusedUDP)
	}
}
