package tunservice

import "testing"

// pkexec's words when no polkit agent can ask (an app started over SSH)
// are told apart from a refusal.
func TestNoPromptAgent(t *testing.T) {
	for out, want := range map[string]bool{
		"Error creating textual authentication agent: Error opening current controlling terminal for the process (`/dev/tty'): No such device or address": true,
		"No authentication agent found.":                          true,
		"Error executing command as another user: Not authorized": false,
		"": false,
	} {
		if got := noPromptAgent(out); got != want {
			t.Errorf("noPromptAgent(%q) = %v, want %v", out, got, want)
		}
	}
}
