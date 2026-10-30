//go:build !windows && !linux && !darwin

package tunservice

import (
	"errors"
	"io"
)

// startFilters keeps DNS inside the adapter; not written for this system,
// so the TUN does not start rather than let DNS leave.
func startFilters(uint32, []string, bool) (io.Closer, error) {
	return nil, errors.New("the DNS filters of the TUN mode are not available on this system")
}
