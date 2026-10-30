package tunapi

import (
	"runtime"
	"testing"
)

func TestProgramsMatchByNameOrPattern(t *testing.T) {
	for _, c := range []struct {
		pattern, name string
		want          bool
	}{
		{"steam.exe", "Steam.EXE", true},
		{"steam.exe", "steamwebhelper.exe", false},
		{"steam*", "steamwebhelper.exe", true},
		{"STEAM*", "steam.exe", true},
		{"*game*", "MyGameClient.exe", true},
		{"tele?ram.exe", "Telegram.exe", true},
		{"steam*", "notsteam.exe", false},
		{"app[1].exe", "app[1].exe", true}, // no * or ?: the name as it is
		{"[*", "x", false},                 // a broken pattern matches nothing
	} {
		if got := MatchProgram(c.pattern, c.name); got != c.want {
			t.Errorf("MatchProgram(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

func TestTheSystemsOwnPrograms(t *testing.T) {
	var cases map[string]bool
	switch runtime.GOOS {
	case "windows":
		t.Setenv("SystemRoot", `C:\Windows`)
		cases = map[string]bool{
			`C:\Windows\System32\svchost.exe`:                    true,
			`c:\windows\system32\svchost.exe`:                    true,
			`C:\Windows`:                                         false,
			`C:\WindowsApps\x.exe`:                               false,
			`C:\Program Files\WindowsApps\Telegram\Telegram.exe`: false,
		}
	case "linux":
		cases = map[string]bool{
			"/usr/sbin/chronyd":                            true,
			"/usr/lib/systemd/systemd-resolved":            true,
			"/usr/bin/telegram-desktop":                    false,
			"/opt/discord/Discord":                         false,
			"/home/u/.local/share/Steam/ubuntu12_32/steam": false,
		}
	case "darwin":
		cases = map[string]bool{
			"/usr/sbin/mDNSResponder":                            true,
			"/usr/libexec/timed":                                 true,
			"/System/Library/CoreServices/Finder.app/x":          true,
			"/Applications/Telegram.app/Contents/MacOS/Telegram": false,
		}
	default:
		t.Skip("no system programs are known here")
	}
	for path, want := range cases {
		if got := SystemProgram(path); got != want {
			t.Errorf("SystemProgram(%q) = %v, want %v", path, got, want)
		}
	}
}
