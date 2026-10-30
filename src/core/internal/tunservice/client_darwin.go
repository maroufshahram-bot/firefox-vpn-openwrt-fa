package tunservice

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runElevated runs this executable with args as root through AppleScript's
// "do shell script ... with administrator privileges", whose prompt asks
// for an administrator's password and shows prompt as the reason, and
// waits for it.
func runElevated(prompt string, args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	report, remove, err := reportFile()
	if err != nil {
		return err
	}
	defer remove()
	words := []string{shellQuote(self)}
	for _, a := range append(args, "-report", report) {
		words = append(words, shellQuote(a))
	}
	script := "do shell script " + appleScriptString(strings.Join(words, " "))
	if prompt != "" {
		script += " with prompt " + appleScriptString(prompt)
	}
	script += " with administrator privileges"
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err == nil {
		return nil
	}
	text := strings.TrimSpace(string(out))
	// "execution error: User canceled. (-128)"
	if strings.HasSuffix(text, "(-128)") {
		return ErrCancelled
	}
	if r := readReport(report); r != "" {
		return errors.New(r)
	}
	if text != "" {
		return fmt.Errorf("running as the administrator failed: %s", text)
	}
	return fmt.Errorf("running as the administrator failed: %w", err)
}

// shellQuote quotes s for sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// appleScriptString quotes s as an AppleScript string.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
