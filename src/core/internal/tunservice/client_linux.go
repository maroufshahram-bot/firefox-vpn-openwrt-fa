package tunservice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pkexec's exit statuses for the prompt (pkexec(1)).
const (
	pkexecDismissed     = 126
	pkexecNotAuthorized = 127
)

// runElevated runs this executable with args as root through pkexec,
// whose polkit prompt asks for the permission, and waits for it. polkit's
// prompt names the program, so prompt (macOS') is not used.
func runElevated(_ string, args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return errors.New("asking for the administrator permission needs pkexec (polkit), which is not found")
	}
	report, remove, err := reportFile()
	if err != nil {
		return err
	}
	defer remove()
	out, err := exec.Command(pkexec, append(append([]string{self}, args...), "-report", report)...).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return err
	}
	switch exit.ExitCode() {
	case pkexecDismissed:
		return ErrCancelled
	case pkexecNotAuthorized:
		if noPromptAgent(string(out)) {
			return fmt.Errorf("no password prompt could open here: the app was not started from the desktop (but from a terminal or over SSH, for one); start Firefox VPN from the applications menu, or install the service in a terminal: sudo %q install", self)
		}
		if text := strings.TrimSpace(string(out)); text != "" {
			return fmt.Errorf("the administrator permission was refused: %s", text)
		}
		return errors.New("the administrator permission was refused")
	}
	if text := readReport(report); text != "" {
		return errors.New(text)
	}
	return fmt.Errorf("%s failed with exit code %d", filepath.Base(self), exit.ExitCode())
}

// noPromptAgent reports whether pkexec found no polkit agent to ask with:
// the program does not belong to a desktop session that runs one (it was
// started from a terminal or over SSH), so pkexec tried the terminal's
// prompt, and there is no terminal either.
func noPromptAgent(output string) bool {
	return strings.Contains(output, "textual authentication agent") ||
		strings.Contains(output, "No authentication agent found")
}
