package tunapi

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

func pipePair(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return NewConn(a), NewConn(b)
}

func TestMessagesGoAcrossOneLineEach(t *testing.T) {
	client, server := pipePair(t)
	sent := []Message{
		Hello("1.2.3"),
		{Type: TypeOpen, Proxy: "127.0.0.1:5000", Username: "u", Password: "p"},
		{Type: TypeOpened, Interface: &Interface{Name: "Firefox VPN", Index: 31, LUID: 1 << 40}},
	}
	go func() {
		for _, m := range sent {
			if err := client.Send(m); err != nil {
				t.Error(err)
			}
		}
		client.Close()
	}()
	for i, want := range sent {
		got, err := server.Receive()
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if got.Type != want.Type || got.Proxy != want.Proxy || got.Version != want.Version ||
			(want.Interface != nil && (got.Interface == nil || *got.Interface != *want.Interface)) {
			t.Fatalf("message %d: got %+v, want %+v", i, got, want)
		}
	}
	if _, err := server.Receive(); !errors.Is(err, io.EOF) {
		t.Fatalf("after the last message: %v, want EOF", err)
	}
}

func TestAMessageLongerThanTheLimitIsRefused(t *testing.T) {
	client, server := pipePair(t)
	if err := client.Send(Message{Type: TypeError, Message: strings.Repeat("x", MaxMessage)}); !errors.Is(err, ErrTooLong) {
		t.Fatalf("sending: %v, want ErrTooLong", err)
	}
	go func() {
		// A peer that ignores the limit.
		_, _ = client.Raw().Write([]byte(`{"type":"error","message":"` + strings.Repeat("x", MaxMessage) + "\"}\n"))
	}()
	if _, err := server.Receive(); !errors.Is(err, ErrTooLong) {
		t.Fatalf("receiving: %v, want ErrTooLong", err)
	}
}

func TestALineThatIsNotAMessageIsAnError(t *testing.T) {
	client, server := pipePair(t)
	go func() { _, _ = client.Raw().Write([]byte("{\"protocol\":1}\nnot json\n")) }()
	for range 2 {
		if _, err := server.Receive(); err == nil {
			t.Fatal("a line without a type or not JSON was accepted")
		}
	}
}

func TestTheCommonProtocolIsTheNewestBothSpeak(t *testing.T) {
	for _, c := range []struct{ protocol, min, want int }{
		{Protocol, MinProtocol, Protocol},
		{Protocol + 5, Protocol, Protocol},
		{Protocol + 5, Protocol + 1, 0}, // the peer dropped ours
		{MinProtocol - 1, MinProtocol - 1, 0},
	} {
		if got := Common(c.protocol, c.min); got != c.want {
			t.Errorf("Common(%d, %d) = %d, want %d", c.protocol, c.min, got, c.want)
		}
	}
}

func TestErrorsAndFailuresBecomeGoErrors(t *testing.T) {
	if AsError(Message{Type: TypeRouted}) != nil {
		t.Fatal("a reply became an error")
	}
	err := AsError(Message{Type: TypeError, Code: CodeProtocol, Protocol: 3, MinProtocol: 2, Version: "2.0.0"})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeProtocol || e.Protocol != 3 || e.MinProtocol != 2 || e.Version != "2.0.0" {
		t.Fatalf("got %#v", err)
	}
	if !strings.Contains(AsError(Message{Type: TypeFailed, Code: CodeAdapterLost, Message: "gone"}).Error(), "gone") {
		t.Fatal("the failure's text is lost")
	}
}
