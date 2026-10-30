package desktop

import (
	"os"
	"strconv"
	"strings"
)

// listsPrograms: runningPrograms works here.
const listsPrograms = true

// listProcesses is the executable of each of this user's processes, from
// /proc; another user's is not readable and is left out, as are kernel
// threads.
func listProcesses() ([]string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if path, err := os.Readlink("/proc/" + e.Name() + "/exe"); err == nil {
			paths = append(paths, strings.TrimSuffix(path, " (deleted)"))
		}
	}
	return paths, nil
}
