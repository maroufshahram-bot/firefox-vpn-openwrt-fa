package desktop

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/armanlife/firefox-vpn-client/core/internal/service"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// The running programs include this test, once each by name, in order of
// their names, without the system's own.
func TestTheRunningProgramsAreListed(t *testing.T) {
	if !listsPrograms {
		t.Skip("no programs are listed here")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	programs, err := runningPrograms()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(programs, func(p service.RunningProgram) bool {
		return strings.EqualFold(p.Name, filepath.Base(self)) && strings.EqualFold(p.Path, self)
	}) {
		t.Fatalf("this test (%s) is not among %d programs", self, len(programs))
	}
	seen := map[string]bool{}
	for i, p := range programs {
		key := strings.ToLower(p.Name)
		if seen[key] {
			t.Errorf("%s twice", p.Name)
		}
		seen[key] = true
		if i > 0 && strings.ToLower(programs[i-1].Name) > key {
			t.Errorf("%s before %s", programs[i-1].Name, p.Name)
		}
		if tunapi.SystemProgram(p.Path) || ownPrograms[key] {
			t.Errorf("%s (%s) is listed", p.Name, p.Path)
		}
	}
}
