package tunservice

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
)

// The filters on macOS: the DNS servers of every network service are the
// TUN's while it runs (macdns.go), and for an adapter without IPv6, IPv6
// outside it is refused by blackhole routes for both halves of the address
// space, more specific than any default route. Both are noted in the
// service's folder before they are made, so that they are undone after a
// crash too (restoreFilters, when the service starts again).

const (
	dnsBackupFile = "dns-backup.json"
	// ipv6BlockedFile is there while the blackhole routes may be.
	ipv6BlockedFile = "ipv6-blocked"
)

var ipv6Halves = []string{"::/1", "8000::/1"}

// dnsRecheck is how often the DNS settings are looked at while the TUN
// runs: a network service added meanwhile (a USB adapter), or one whose
// setting changed, gets the TUN's DNS server again.
const dnsRecheck = 10 * time.Second

func systemDNS() macDNS {
	return macDNS{
		run: func(args ...string) (string, error) {
			out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
			text := strings.TrimSpace(string(out))
			if err != nil && text != "" {
				return text, fmt.Errorf("networksetup %s: %s", args[0], text)
			}
			return text, err
		},
		backup: filepath.Join(stateDir, dnsBackupFile),
		server: adapterV4.Addr().Next().String(),
	}
}

// startFilters sets the TUN's DNS server on every network service, and
// blocks IPv6 outside an adapter that has none.
func startFilters(_ uint32, _ []string, blockIPv6 bool) (io.Closer, error) {
	if blockIPv6 {
		if err := os.WriteFile(filepath.Join(stateDir, ipv6BlockedFile), nil, 0o644); err != nil {
			return nil, err
		}
		for _, half := range ipv6Halves {
			if err := run("/sbin/route", "-n", "add", "-inet6", "-net", half, "::1", "-blackhole"); err != nil {
				return nil, errors.Join(fmt.Errorf("blocking IPv6: %w", err), restoreFilters())
			}
		}
	}
	dns := systemDNS()
	if err := dns.apply(); err != nil {
		return nil, errors.Join(fmt.Errorf("setting the TUN's DNS server: %w", err), restoreFilters())
	}
	flushDNSCache()
	f := &filters{dns: dns, stop: make(chan struct{}), done: make(chan struct{})}
	go f.watch()
	return f, nil
}

type filters struct {
	dns  macDNS
	stop chan struct{}
	done chan struct{}
	once sync.Once
	err  error
}

func (f *filters) watch() {
	defer close(f.done)
	t := time.NewTicker(dnsRecheck)
	defer t.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-t.C:
			if err := f.dns.apply(); err != nil {
				logx.Warn("setting the TUN's DNS server again failed err=%s", logx.Err(err))
			}
		}
	}
}

func (f *filters) Close() error {
	f.once.Do(func() {
		close(f.stop)
		<-f.done
		f.err = restoreFilters()
	})
	return f.err
}

// restoreFilters undoes what startFilters did, also when a crash kept it
// from doing so.
func restoreFilters() error {
	var errs []error
	blocked := filepath.Join(stateDir, ipv6BlockedFile)
	if _, err := os.Stat(blocked); err == nil {
		for _, half := range ipv6Halves {
			// Not there when adding it failed.
			_ = run("/sbin/route", "-n", "delete", "-inet6", "-net", half)
		}
		if err := os.Remove(blocked); err != nil {
			errs = append(errs, err)
		}
	}
	if err := systemDNS().restore(); err != nil {
		errs = append(errs, fmt.Errorf("setting back the DNS servers: %w", err))
	}
	flushDNSCache()
	return errors.Join(errs...)
}

// flushDNSCache drops the answers macOS keeps: the network's real ones
// when the TUN starts, the TUN's fake ones (a minute's) when it stops.
func flushDNSCache() {
	_ = run("/usr/bin/dscacheutil", "-flushcache")
	_ = run("/usr/bin/killall", "-HUP", "mDNSResponder")
}
