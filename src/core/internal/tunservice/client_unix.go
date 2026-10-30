//go:build linux || darwin

package tunservice

import (
	"os"
	"strconv"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// InstallForUser installs the service for the user running this process
// (as root through sudo, for the user who ran sudo), asking for the
// administrator's permission; prompt is the reason macOS' prompt shows.
func InstallForUser(prompt string) error {
	uid := strconv.Itoa(os.Getuid())
	if os.Geteuid() == 0 {
		if sudo := os.Getenv("SUDO_UID"); sudo != "" && uid == "0" {
			uid = sudo
		}
		return Install(uid)
	}
	return runElevated(prompt, "install-as-admin", "-user", uid)
}

// UninstallForUser removes the service: through its socket while it runs,
// with no prompt; else as the administrator.
func UninstallForUser(prompt string) error {
	if installed, err := tunapi.Installed(); err == nil && !installed {
		return nil
	}
	if err := askService(tunapi.Message{Type: tunapi.TypeUninstall}, tunapi.TypeUninstalling); err == nil {
		return waitRemoved(30 * time.Second)
	}
	if os.Geteuid() == 0 {
		return Uninstall()
	}
	return runElevated(prompt, "uninstall-as-admin")
}

// reportFile makes the file an elevated copy writes why it failed to, and
// returns a function that removes it.
func reportFile() (string, func(), error) {
	f, err := os.CreateTemp("", "firefox-vpn-tun-*.txt")
	if err != nil {
		return "", nil, err
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}
