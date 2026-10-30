package tunservice

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeDNSSetup answers like networksetup for Wi-Fi (the user's own DNS
// server), Ethernet (the network's), and a disabled Bluetooth PAN.
type fakeDNSSetup struct {
	dns   map[string][]string
	calls []string
	// fail makes setting this service's servers fail, the way networksetup
	// does: a message, and no error.
	fail string
}

func newFakeDNSSetup(t *testing.T) (*fakeDNSSetup, macDNS) {
	f := &fakeDNSSetup{dns: map[string][]string{
		"Wi-Fi":         {"1.1.1.1", "2606:4700:4700::1111"},
		"Ethernet":      nil,
		"Bluetooth PAN": nil,
	}}
	return f, macDNS{run: f.run, backup: filepath.Join(t.TempDir(), "dns-backup.json"), server: "198.18.0.2"}
}

func (f *fakeDNSSetup) run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if args[0] == "-listallnetworkservices" {
		return "An asterisk (*) denotes that a network service is disabled.\nWi-Fi\nEthernet\n*Bluetooth PAN", nil
	}
	servers, ok := f.dns[args[1]]
	if !ok {
		return "** Error: " + args[1] + " is not a recognized network service.", nil
	}
	switch args[0] {
	case "-getdnsservers":
		if len(servers) == 0 {
			return "There aren't any DNS Servers set on " + args[1] + ".", nil
		}
		return strings.Join(servers, "\n"), nil
	case "-setdnsservers":
		if args[1] == f.fail {
			return "** Error: The parameters were not valid.", nil
		}
		if args[2] == "Empty" {
			f.dns[args[1]] = nil
		} else {
			f.dns[args[1]] = append([]string(nil), args[2:]...)
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected %v", args)
}

func TestTheTunsDNSServerIsSetOnEveryServiceAndSetBack(t *testing.T) {
	f, m := newFakeDNSSetup(t)
	if err := m.apply(); err != nil {
		t.Fatal(err)
	}
	for s, servers := range f.dns {
		if !slices.Equal(servers, []string{"198.18.0.2"}) {
			t.Errorf("%s has %v", s, servers)
		}
	}
	if _, err := os.Stat(m.backup); err != nil {
		t.Fatalf("no backup: %v", err)
	}
	// Checked again while the TUN runs: nothing to change.
	f.calls = nil
	if err := m.apply(); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "-setdnsservers") {
			t.Errorf("set again: %s", c)
		}
	}

	if err := m.restore(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dns["Wi-Fi"], []string{"1.1.1.1", "2606:4700:4700::1111"}) || f.dns["Ethernet"] != nil || f.dns["Bluetooth PAN"] != nil {
		t.Fatalf("set back to %v", f.dns)
	}
	if _, err := os.Stat(m.backup); !os.IsNotExist(err) {
		t.Fatalf("the backup is still there: %v", err)
	}
}

// A crash leaves the TUN's DNS server and the backup: the service sets
// them back when it starts again, with a new macDNS.
func TestTheDNSServersAreSetBackAfterACrash(t *testing.T) {
	f, m := newFakeDNSSetup(t)
	if err := m.apply(); err != nil {
		t.Fatal(err)
	}
	again := macDNS{run: f.run, backup: m.backup, server: m.server}
	if err := again.restore(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dns["Wi-Fi"], []string{"1.1.1.1", "2606:4700:4700::1111"}) || f.dns["Ethernet"] != nil {
		t.Fatalf("set back to %v", f.dns)
	}
	// Without a backup there is nothing to do.
	f.calls = nil
	if err := again.restore(); err != nil || len(f.calls) != 0 {
		t.Fatalf("restore without a backup: %v, calls %v", err, f.calls)
	}
}

// A setting the user changes while the TUN runs is set back to what they
// chose, not to what it was before; one they change after stays.
func TestADNSSettingChangedMeanwhileIsKept(t *testing.T) {
	f, m := newFakeDNSSetup(t)
	if err := m.apply(); err != nil {
		t.Fatal(err)
	}
	f.dns["Wi-Fi"] = []string{"9.9.9.9"}
	if err := m.apply(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dns["Wi-Fi"], []string{"198.18.0.2"}) {
		t.Fatalf("Wi-Fi has %v while the TUN runs", f.dns["Wi-Fi"])
	}
	f.dns["Ethernet"] = []string{"8.8.8.8"}
	if err := m.restore(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dns["Wi-Fi"], []string{"9.9.9.9"}) || !slices.Equal(f.dns["Ethernet"], []string{"8.8.8.8"}) {
		t.Fatalf("set back to %v", f.dns)
	}
}

// networksetup reports some failures only in its output.
func TestAFailedDNSSettingIsAnError(t *testing.T) {
	f, m := newFakeDNSSetup(t)
	f.fail = "Ethernet"
	err := m.apply()
	if err == nil || !strings.Contains(err.Error(), "Ethernet") {
		t.Fatalf("got %v", err)
	}
	// What was changed is still set back.
	f.fail = ""
	if err := m.restore(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.dns["Wi-Fi"], []string{"1.1.1.1", "2606:4700:4700::1111"}) {
		t.Fatalf("Wi-Fi has %v", f.dns["Wi-Fi"])
	}
	if _, err := m.get("Nowhere"); err == nil {
		t.Fatal("reading an unknown service succeeded")
	}
}
