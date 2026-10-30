package desktop

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/service"
)

func TestConsoleMenuItems(t *testing.T) {
	actions := func(items []menuItem) string {
		var names []string
		for _, it := range items {
			names = append(names, it.action)
		}
		return strings.Join(names, ",")
	}

	// Not reachable: offer to start, never to exit.
	ui := &consoleUI{lastErr: "connection refused"}
	if got := actions(ui.menuItems()); !strings.Contains(got, actStart) || strings.Contains(got, actExit) {
		t.Fatalf("stopped menu = %s", got)
	}
	// Running in the tray: exit completely is the last numbered item.
	ui = &consoleUI{status: &service.StatusView{CanExit: true}}
	got := actions(ui.menuItems())
	if !strings.Contains(got, actLocation) || !strings.HasSuffix(got, actExit) {
		t.Fatalf("tray menu = %s", got)
	}
	// A copy that no tray or app hosts (another terminal, systemd) cannot
	// be exited from here.
	ui = &consoleUI{status: &service.StatusView{CanExit: false}}
	if got := actions(ui.menuItems()); strings.Contains(got, actExit) {
		t.Fatalf("a copy that cannot exit must not offer exit: %s", got)
	}
}

func TestConsoleNumberPrompt(t *testing.T) {
	ui := &consoleUI{status: &service.StatusView{CanExit: true}}
	logIndex := 0
	for i, it := range ui.menuItems() {
		if it.action == actLog {
			logIndex = i + 1
		}
	}
	typeLine := func(text string) {
		for _, r := range text {
			ui.handleKey(r)
		}
		ui.handleKey('\r')
	}

	// Persian digits, Backspace, and Enter.
	persian := []rune("۰۱۲۳۴۵۶۷۸۹")
	ui.handleKey('9')
	ui.handleKey(127)
	typeLine(string(persian[logIndex]))
	if ui.mode != screenLogs {
		t.Fatalf("entering %d should open the log, mode = %d", logIndex, ui.mode)
	}
	typeLine("0")
	if ui.mode != screenHome {
		t.Fatal("0 should go back from the log")
	}

	typeLine("42")
	if !strings.Contains(ui.message, "Enter a number from 0 to") {
		t.Fatalf("an out-of-range number should explain the range, got %q", ui.message)
	}
	ui.handleKey('7')
	ui.handleKey(27) // Esc clears the input
	if ui.input != "" {
		t.Fatalf("Esc should clear the input, got %q", ui.input)
	}
	typeLine("0")
	if !ui.isQuitting() {
		t.Fatal("0 on the main menu closes the window")
	}
}

func TestConsoleKeysAndWidth(t *testing.T) {
	for in, want := range map[rune]rune{'۳': '3', '٧': '7', '۰': '0', '5': '5', 'x': 'x'} {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
	colored := ansiGreen + "connected" + ansiReset + " now"
	if visibleLen(colored) != len("connected now") {
		t.Fatalf("visibleLen = %d", visibleLen(colored))
	}
	cut := fitWidth(colored, 6)
	if visibleLen(cut) != 6 || !strings.HasSuffix(strings.TrimSuffix(cut, ansiReset), "…") {
		t.Fatalf("fitWidth = %q", cut)
	}
}

// The log screen fills the window but keeps the prompt on it.
func TestLogScreenKeepsThePromptVisible(t *testing.T) {
	ui := &consoleUI{}
	for i := 0; i < 200; i++ {
		ui.logLines = append(ui.logLines, logx.Entry{ID: uint64(i), Time: time.Now(), Level: "INFO", Message: "line"})
	}
	for _, height := range []int{12, 25, 50} {
		lines := ui.logsScreen(80, height)
		if len(lines) > height {
			t.Fatalf("height %d: %d lines", height, len(lines))
		}
		if !strings.Contains(lines[len(lines)-2], "Enter a number") {
			t.Fatalf("height %d: the prompt is not the second-last line", height)
		}
	}
}

// A result shown on one screen does not follow to the next one.
func TestScreenChangeClearsTheMessage(t *testing.T) {
	ui := &consoleUI{status: &service.StatusView{CanExit: true}}
	logIndex := 0
	for i, it := range ui.menuItems() {
		if it.action == actLog {
			logIndex = i + 1
		}
	}
	ui.setMessage("Connection works: 820 ms", ansiGreen)
	for _, r := range strconv.Itoa(logIndex) + "\r" {
		ui.handleKey(r)
	}
	if ui.mode != screenLogs || ui.message != "" {
		t.Fatalf("mode %d, message %q", ui.mode, ui.message)
	}
	// A running task keeps its message.
	ui.busy, ui.message = true, "Testing…"
	ui.handleKey(27)
	if ui.mode != screenHome || ui.message != "Testing…" {
		t.Fatalf("mode %d, message %q", ui.mode, ui.message)
	}
}

// resetBackend answers the panel login screen and the poll; other calls
// are not made.
type resetBackend struct {
	consoleBackend
	sets []string // "username password" asked for
}

// status keeps the menu as it is (the background poll after a task asks).
func (b *resetBackend) status(context.Context) (service.StatusView, error) {
	return service.StatusView{CanExit: true, Dashboard: map[string]any{"login_required": true}}, nil
}
func (b *resetBackend) logsSince(context.Context, uint64) ([]logx.Entry, error) { return nil, nil }
func (b *resetBackend) setLogin(_ context.Context, username, password string) (dashboardLogin, error) {
	b.sets = append(b.sets, username+" "+password)
	if username == "" {
		username, password = "panel2345", "Secret12345678ab"
	}
	return dashboardLogin{Username: username, Password: password}, nil
}

// waitLogin waits until the login set shows.
func waitLogin(t *testing.T, ui *consoleUI) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ui.mu.Lock()
		done := ui.newLogin != nil && !ui.busy
		ui.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return strings.Join(ui.resetLoginScreen(80, 30), "\n")
}

// The panel login: one menu item; a random login, shown once and
// forgotten when the screen is left, or the user's own, typed with the
// password hidden and not shown back.
func TestResetLoginScreen(t *testing.T) {
	backend := &resetBackend{}
	ui := &consoleUI{backend: backend, status: &service.StatusView{CanExit: true, Dashboard: map[string]any{"login_required": true}}}
	index := 0
	for i, it := range ui.menuItems() {
		if it.action == actReset {
			index = i + 1
			if it.label != "Set panel login" {
				t.Fatalf("label %q", it.label)
			}
		}
	}
	if index == 0 {
		t.Fatal("no panel login item")
	}
	ui.submit(strconv.Itoa(index))
	if ui.mode != screenResetLogin || len(backend.sets) != 0 {
		t.Fatal("choosing the item must ask first, not set")
	}
	if text := strings.Join(ui.resetLoginScreen(80, 30), "\n"); !strings.Contains(text, "Random username and password") || !strings.Contains(text, "Choose my own") {
		t.Fatalf("choice screen:\n%s", text)
	}
	ui.submit("1")
	text := waitLogin(t, ui)
	if len(backend.sets) != 1 || backend.sets[0] != " " || !strings.Contains(text, "panel2345") || !strings.Contains(text, "Secret12345678ab") {
		t.Fatalf("random (sets %q):\n%s", backend.sets, text)
	}
	ui.submit("0")
	if ui.mode != screenHome || ui.newLogin != nil {
		t.Fatal("leaving the screen must forget the password")
	}

	// The user's own: a short password is refused, the typed one is
	// hidden and not shown back.
	ui.submit(strconv.Itoa(index))
	ui.submit("2")
	typeLine := func(text string) {
		for _, r := range text {
			ui.handleKey(r)
		}
		ui.handleKey('\r')
	}
	typeLine("me")
	for _, r := range "secret" {
		ui.handleKey(r)
	}
	ui.mu.Lock()
	prompt := ui.promptLine()
	ui.mu.Unlock()
	if strings.Contains(prompt, "secret") || !strings.Contains(prompt, "••••••") {
		t.Fatalf("the password must be hidden: %q", prompt)
	}
	ui.handleKey('\r')
	text = waitLogin(t, ui)
	if len(backend.sets) != 2 || backend.sets[1] != "me secret" || !strings.Contains(text, "me") || strings.Contains(text, "secret") {
		t.Fatalf("own (sets %q):\n%s", backend.sets, text)
	}
	ui.submit("0")
	ui.submit(strconv.Itoa(index))
	ui.submit("2")
	typeLine("you")
	typeLine("short")
	if len(backend.sets) != 2 || !strings.Contains(ui.message, "at least 6") {
		t.Fatalf("a short password: sets %q, message %q", backend.sets, ui.message)
	}
}

func TestProxyAccessSaysWhoCanConnect(t *testing.T) {
	for _, c := range []struct {
		listen string
		login  bool
		want   string
	}{
		{"127.0.0.1:10810", false, "SOCKS5 and HTTP · no password · this computer only"},
		{"0.0.0.0:10810", true, "SOCKS5 and HTTP · username and password · open to the network"},
		{"[::1]:10810", false, "SOCKS5 and HTTP · no password · this computer only"},
	} {
		if got := proxyAccess(c.listen, c.login); got != c.want {
			t.Errorf("%s %v: %q", c.listen, c.login, got)
		}
	}
}

func TestDashboardHowToWithoutABrowser(t *testing.T) {
	got := dashboardHowTo("127.0.0.1:10820", []string{"203.0.113.5"})
	if !strings.Contains(got, "ssh -L 10820:127.0.0.1:10820") || !strings.Contains(got, "http://127.0.0.1:10820") {
		t.Fatalf("loopback: %q", got)
	}
	got = dashboardHowTo("0.0.0.0:10820", []string{"203.0.113.5", "10.0.0.2"})
	if !strings.Contains(got, "http://203.0.113.5:10820") || !strings.Contains(got, "http://10.0.0.2:10820") {
		t.Fatalf("shared: %q", got)
	}
}
