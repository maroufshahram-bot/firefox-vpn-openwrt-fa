//go:build !windows && !linux

package desktop

import "errors"

// listsPrograms: no program skips the VPN here (macOS: the TUN service
// does not find connections' programs), so none are listed.
const listsPrograms = false

func listProcesses() ([]string, error) { return nil, errors.ErrUnsupported }
