package tunservice

import (
	"net/netip"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func dnsQuery(t *testing.T, name string) []byte {
	t.Helper()
	msg := dnsmessage.Message{
		Header:    dnsmessage.Header{ID: 1, RecursionDesired: true},
		Questions: []dnsmessage.Question{{Name: dnsmessage.MustNewName(name), Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}},
	}
	b, err := msg.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// lastA is the last A record of a DNS reply.
func lastA(t *testing.T, reply []byte) netip.Addr {
	t.Helper()
	var m dnsmessage.Message
	if err := m.Unpack(reply); err != nil {
		t.Fatal(err)
	}
	var addr netip.Addr
	for _, a := range m.Answers {
		if r, ok := a.Body.(*dnsmessage.AResource); ok {
			addr = netip.AddrFrom4(r.A)
		}
	}
	if !addr.IsValid() {
		t.Fatalf("no A record in %+v", m)
	}
	return addr
}
