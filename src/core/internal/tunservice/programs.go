package tunservice

import (
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/tunapi"
)

// Programs that skip the VPN (tunapi.Program, the core's tun.programs): the
// service finds the program behind an app's new connection by its address
// on the adapter (findProgram: the system's table of sockets), and sends a
// chosen program's traffic directly, outside the TUN (egress), instead of to
// the core: its TCP and UDP, or its UDP alone. Mozilla carries no UDP, so
// every other program's UDP is refused; a refused call or game is told to
// the core once per program (tellRefused), so the app can offer to send it
// directly.

// programSet is the programs that skip the VPN: by lower-case file name,
// each to whether only its UDP skips it, and the patterns (with * or ?) in
// their order. A name wins over a pattern.
type programSet struct {
	names    map[string]bool
	patterns []tunapi.Program
}

func newProgramSet(programs []tunapi.Program) programSet {
	set := programSet{names: map[string]bool{}}
	for _, p := range programs {
		switch name := strings.TrimSpace(p.Name); {
		case name == "":
		case strings.ContainsAny(name, "*?"):
			set.patterns = append(set.patterns, tunapi.Program{Name: name, UDPOnly: p.UDPOnly})
		default:
			set.names[strings.ToLower(name)] = p.UDPOnly
		}
	}
	return set
}

func (s programSet) size() int { return len(s.names) + len(s.patterns) }

// lookup reports whether the program named name skips the VPN, and whether
// only its UDP does.
func (s programSet) lookup(name string) (udpOnly, ok bool) {
	if name == "" {
		return false, false
	}
	if udpOnly, ok := s.names[strings.ToLower(name)]; ok {
		return udpOnly, true
	}
	for _, p := range s.patterns {
		if tunapi.MatchProgram(p.Name, name) {
			return p.UDPOnly, true
		}
	}
	return false, false
}

// programName is the file name of an executable's path.
func programName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// findProgram is the executable of the program whose socket has source as
// its local address ("" with an error when none is found); tests replace
// it.
var findProgram = ownerPath

// owners keeps the programs of UDP ports a moment: a port whose UDP is
// refused is asked about again with each packet.
type owners struct {
	mu   sync.Mutex
	seen map[netip.AddrPort]owner
}

type owner struct {
	path string
	at   time.Time
}

const (
	// ownerTTL is how long a UDP port's program is kept.
	ownerTTL = 5 * time.Second
	// maxOwners bounds owners.
	maxOwners = 4096
)

// udp is the program of the UDP port source.
func (o *owners) udp(source netip.AddrPort) string {
	now := time.Now()
	o.mu.Lock()
	if e, ok := o.seen[source]; ok && now.Sub(e.at) < ownerTTL {
		o.mu.Unlock()
		return e.path
	}
	o.mu.Unlock()
	path := lookProgram(true, source)
	o.mu.Lock()
	if o.seen == nil || len(o.seen) >= maxOwners {
		o.seen = map[netip.AddrPort]owner{}
	}
	o.seen[source] = owner{path: path, at: now}
	o.mu.Unlock()
	return path
}

// lookProgram is findProgram, "" when it finds none.
func lookProgram(udp bool, source netip.AddrPort) string {
	path, err := findProgram(udp, source)
	if err != nil {
		logx.Debug("TUN: no program found for source=%s udp=%v err=%s", source, udp, logx.Err(err))
		return ""
	}
	return path
}

// programSet is the session's programs that skip the VPN.
func (s *session) programSet() programSet {
	if p := s.programs.Load(); p != nil {
		return *p
	}
	return programSet{}
}

// setPrograms replaces the programs that skip the VPN: for the TCP
// connections that open from now on, and every UDP flow, which is judged
// again (a program just added may call at once; one just removed stops).
func (s *session) setPrograms(programs []tunapi.Program) {
	set := newProgramSet(programs)
	s.programs.Store(&set)
	if s.dev != nil {
		s.dev.ResetFlows()
	}
	logx.Info("TUN: programs that skip the VPN count=%d", set.size())
}

// directTCP reports whether the TCP connection from source is sent
// directly: its program's whole traffic skips the VPN.
func (s *session) directTCP(source netip.AddrPort) bool {
	set := s.programSet()
	if set.size() == 0 {
		return false
	}
	name := programName(lookProgram(false, source))
	if udpOnly, ok := set.lookup(name); ok && !udpOnly {
		logx.Debug("TUN: program=%s connects directly", name)
		return true
	}
	return false
}

// takeUDP decides on a port's first UDP packet to dst: it is taken (sent
// directly) when its program skips the VPN, except QUIC (port 443) of a
// program whose UDP alone skips it, which is refused so that it falls back
// to TCP through the VPN; everything else is refused, and a call or a game
// told to the core.
func (s *session) takeUDP(source, dst netip.AddrPort) bool {
	set := s.programSet()
	call := callLike(dst)
	if set.size() == 0 && !call {
		return false
	}
	path := s.owners.udp(source)
	name := programName(path)
	if udpOnly, ok := set.lookup(name); ok {
		return !udpOnly || dst.Port() != 443
	}
	if call && name != "" && !tunapi.SystemProgram(path) {
		s.tellRefused(name, path)
	}
	return false
}

// callLike reports whether UDP to dst looks like a call or a game: to an
// address on the internet (a fake one stands for a name), and not QUIC
// (443), DNS (53, 853), or the clock (123).
func callLike(dst netip.AddrPort) bool {
	switch dst.Port() {
	case 53, 123, 443, 853:
		return false
	}
	addr := dst.Addr().Unmap()
	return addr.IsGlobalUnicast() && !addr.IsPrivate()
}

// tellRefused tells the core, once per program in the session, that a
// program's UDP that looks like a call or a game was refused (protocol 2),
// with the program's executable. Until the TUN is routed the core waits for
// its replies, so nothing is sent then; the next refused packet tells it.
func (s *session) tellRefused(name, path string) {
	if s.protocol < 2 || !s.live.Load() {
		return
	}
	key := strings.ToLower(name)
	s.toldMu.Lock()
	if s.told[key] {
		s.toldMu.Unlock()
		return
	}
	if s.told == nil {
		s.told = map[string]bool{}
	}
	s.told[key] = true
	s.toldMu.Unlock()
	logx.Info("TUN: UDP of program=%s refused (Mozilla carries no UDP)", name)
	// Off the packet path: the core may be slow to read.
	go func() { _ = s.owner.Send(tunapi.Message{Type: tunapi.TypeUDPRefused, Program: name, Path: path}) }()
}
