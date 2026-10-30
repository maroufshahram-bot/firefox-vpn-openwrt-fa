package tunapi

import (
	"os"
	"path"
	"runtime"
	"strings"
)

// MatchProgram reports whether an executable's file name is the program a
// Program's Name stands for: the same name, without case, or a pattern
// with * (any text) and ? (one character), such as steam*.
func MatchProgram(pattern, name string) bool {
	pattern, name = strings.ToLower(pattern), strings.ToLower(name)
	if !strings.ContainsAny(pattern, "*?") {
		return pattern == name
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// SystemProgram reports whether path is one of the system's own programs:
// on Windows one in its folder (its services' svchost, for one), on Linux
// and macOS a system daemon (systemd's, the clock's, mDNSResponder). Its
// UDP is no call, and no one picks it to bypass the VPN.
func SystemProgram(path string) bool {
	switch runtime.GOOS {
	case "windows":
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		prefix := strings.TrimRight(root, `\`) + `\`
		return len(path) > len(prefix) && strings.EqualFold(path[:len(prefix)], prefix)
	case "linux":
		for _, dir := range []string{"/usr/sbin/", "/sbin/", "/usr/lib/systemd/", "/lib/systemd/", "/usr/libexec/"} {
			if strings.HasPrefix(path, dir) {
				return true
			}
		}
	case "darwin":
		for _, dir := range []string{"/System/", "/usr/sbin/", "/sbin/", "/usr/libexec/"} {
			if strings.HasPrefix(path, dir) {
				return true
			}
		}
	}
	return false
}
