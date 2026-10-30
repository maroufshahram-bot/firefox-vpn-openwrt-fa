package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// listsPrograms: runningPrograms works here.
const listsPrograms = true

// listProcesses is the executable of each process, from Windows' list of
// them; one of another user, or a protected one, has no path to read and
// is left out.
func listProcesses() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var paths []string
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		// The idle process and the kernel have no executable.
		if entry.ProcessID == 0 || entry.ProcessID == 4 {
			continue
		}
		if path, err := tunapi.ProcessPath(entry.ProcessID); err == nil && path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
