package tunservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
)

// The DNS servers of macOS's network services while the TUN runs
// (filters_darwin.go). The routes alone would let DNS to a server on the
// local network (the router's, usually) leave outside the TUN, and macOS
// has no filters like Windows' to stop it: every network service's DNS
// servers are set to the TUN's while it runs, as wg-quick does, and set
// back afterwards. What each service had is written to a backup file first,
// so that a service that crashed sets it back when it starts again. It is
// plain code without a build tag, so its tests run everywhere.

// macDNS sets the DNS servers with networksetup; tests replace run.
type macDNS struct {
	run func(args ...string) (string, error)
	// backup is the backup file.
	backup string
	// server is the TUN's DNS server.
	server string
}

// macDNSBackup is what each service had: its DNS servers, or none for
// networksetup's "Empty" (the network's own).
type macDNSBackup map[string][]string

// services lists every network service, the disabled ones too (one may be
// turned on while the TUN runs).
func (m macDNS) services() ([]string, error) {
	out, err := m.run("-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	var names []string
	for i, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// The first line explains the asterisk, which marks a disabled
		// service.
		if line == "" || (i == 0 && strings.Contains(line, "asterisk")) {
			continue
		}
		names = append(names, strings.TrimPrefix(line, "*"))
	}
	if len(names) == 0 {
		return nil, errors.New("no network service")
	}
	return names, nil
}

// get reads a service's DNS servers: none when it has the network's own.
func (m macDNS) get(service string) ([]string, error) {
	out, err := m.run("-getdnsservers", service)
	if err != nil {
		return nil, err
	}
	var servers []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "There aren't any DNS Servers set on Wi-Fi."
		if strings.Contains(line, "aren't any DNS Servers") {
			return nil, nil
		}
		if _, err := netip.ParseAddr(line); err != nil {
			return nil, fmt.Errorf("reading the DNS servers of %s: %s", service, out)
		}
		servers = append(servers, line)
	}
	return servers, nil
}

// set gives a service the DNS servers, or the network's own for none.
func (m macDNS) set(service string, servers []string) error {
	args := append([]string{"-setdnsservers", service}, servers...)
	if len(servers) == 0 {
		args = append(args, "Empty")
	}
	out, err := m.run(args...)
	if err != nil {
		return err
	}
	// networksetup says what failed, but does not always exit with an
	// error.
	if strings.Contains(out, "Error") || strings.Contains(out, "not a recognized") {
		return fmt.Errorf("setting the DNS servers of %s: %s", service, out)
	}
	return nil
}

func (m macDNS) ours(servers []string) bool {
	return len(servers) == 1 && servers[0] == m.server
}

// apply gives every service the TUN's DNS server, noting first what it had.
// A setting changed while the TUN runs is noted again, and set back after.
func (m macDNS) apply() error {
	services, err := m.services()
	if err != nil {
		return err
	}
	backup, _, err := m.loadBackup()
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range services {
		current, err := m.get(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if m.ours(current) {
			continue
		}
		backup[s] = current
		if err := m.saveBackup(backup); err != nil {
			return err
		}
		if err := m.set(s, []string{m.server}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// restore sets back what apply changed: each service that still has the
// TUN's DNS server gets what it had. The backup is removed once all went
// well.
func (m macDNS) restore() error {
	backup, found, err := m.loadBackup()
	if err != nil || !found {
		return err
	}
	services, err := m.services()
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range services {
		current, err := m.get(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		// A setting the user chose meanwhile stays.
		if !m.ours(current) {
			continue
		}
		if err := m.set(s, backup[s]); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := os.Remove(m.backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// loadBackup reads the backup, and whether there is one.
func (m macDNS) loadBackup() (macDNSBackup, bool, error) {
	backup := macDNSBackup{}
	data, err := os.ReadFile(m.backup)
	if errors.Is(err, os.ErrNotExist) {
		return backup, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, true, fmt.Errorf("reading %s: %w", m.backup, err)
	}
	return backup, true, nil
}

func (m macDNS) saveBackup(backup macDNSBackup) error {
	data, err := json.Marshal(backup)
	if err != nil {
		return err
	}
	return replaceFile(m.backup, data, 0o644)
}
