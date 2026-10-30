// Package update installs new versions of the desktop app on Windows,
// Linux, and macOS. The app finds a newer release on GitHub and asks its
// core to download it: the core fetches the release's SHA256SUMS.txt and
// its signature (SHA256SUMS.txt.sig, Ed25519, checked against the key built
// into the core, internal/signing), then the release's zip (Windows) or
// tar.gz (Linux, for the processor; macOS, the app's bundle), whose SHA-256
// must be the one in the signed list. Through the proxy while it is on, as
// GitHub may be blocked; directly otherwise. On Linux the app lives in the
// user's folder (the desktop install of install.sh), and on macOS in a
// folder the user may change (Applications, for an administrator), so no
// administrator is asked.
//
// To install, the core unpacks the package into the update folder, copies
// itself there, and starts the copy with -update (Apply); the app and the
// core then quit as usual (the system proxy is restored). The copy waits
// until both have exited and swaps the app's files: each of the package's
// files and folders replaces the one of the same name, all by renames on
// one drive, and the replaced ones are kept aside to be put back on any
// failure (a file a virus scanner holds, say). The data folder (user-data)
// and anything else in the app's folder stay; on macOS the app's folder is
// its bundle, whose Contents is replaced. For a copy that Setup
// installed, it also sets the version in Windows' list of installed apps;
// the uninstaller removes the app's files whichever version put them there
// ([UninstallDelete] in app/windows/installer/firefox-vpn.iss). The new
// Setup is not run: when it fails, Inno Setup's rollback removes the files
// it added but does not restore the ones it replaced (tried), which can
// leave half of each version. The copy writes what happened to result.json
// and starts the app again; the next core reads the result (Status.Last),
// deletes the copy and the old files, and turns the proxy on again if it
// was on.
//
// Everything lives in the data folder's "update" folder:
//
//	<version>/            the release's signed list and package, once downloaded
//	new/                  the unpacked package, while it is installed
//	backup/               the replaced files, until the next start
//	updater/              the copy of the core that runs -update
//	pending.json          what the copy is to do
//	result.json           what it did, for the next start
//
// While the copy installs, it holds a lock on the data folder (a mutex on
// Windows, update.lock beside update/ elsewhere), so a core that uses the
// same folder and starts meanwhile leaves these files alone.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/logx"
	"github.com/armanlife/firefox-vpn-client/core/internal/signing"
	"github.com/armanlife/firefox-vpn-client/core/internal/version"
)

// The states of Status.
const (
	StateIdle        = "idle"
	StateDownloading = "downloading"
	StateReady       = "ready"
	StateInstalling  = "installing"
	StateFailed      = "failed"
)

// Release files: the signed list and its signature.
const (
	SumsFile = "SHA256SUMS.txt"
	SigFile  = SumsFile + ".sig"
)

// appPackage is how the app comes in a release: the file of a version, the
// app and its core inside it (relative to the app's folder, with "/"), and
// how the file unpacks.
type appPackage struct {
	name      func(v string) string
	app, core string
	unpack    func(file, dir string) error
}

// windowsPackage is the Windows app's zip.
var windowsPackage = appPackage{
	name:   func(v string) string { return "Firefox-VPN-" + v + "-Windows-x64.zip" },
	app:    "firefox-vpn.exe",
	core:   "core/firefox-vpn-core.exe",
	unpack: unzip,
}

// thePackage is this system's: the Windows zip, or the Linux or macOS
// archive (untar.go; process_linux.go and process_darwin.go set it).
var thePackage = windowsPackage

// AppDir is the app's folder for the core at exe: the folder that holds
// the package's core (core/ on Windows and Linux, Contents/MacOS in the
// macOS bundle, whose folder is the bundle).
func AppDir(exe string) string {
	dir := exe
	for range strings.Split(thePackage.core, "/") {
		dir = filepath.Dir(dir)
	}
	return dir
}

// releasesURL is where GitHub serves a release's files:
// <releasesURL>v<version>/<file>.
const releasesURL = "https://github.com/armanlife/firefox-vpn-client/releases/download/"

// ServerEnv names a release server to use instead of GitHub, for trying
// updates with a local test release: it serves
// <server>/releases/download/v<version>/<file>. A loopback server is
// reached directly.
const ServerEnv = "FIREFOX_VPN_UPDATE_SERVER"

// Size limits of what a download reads.
const (
	maxSumsSize    = 64 << 10
	maxSigSize     = 1 << 10
	maxPackageSize = 512 << 20
)

// Download timeouts: the small files, and the package.
const (
	listTimeout    = 30 * time.Second
	packageTimeout = 20 * time.Minute
)

// AssetName is the release's file of version v for this system.
func AssetName(v string) string { return thePackage.name(v) }

// Status is the update's state, for the app (GET /api/status, "update").
type Status struct {
	State string `json:"state"`
	// Version is the version downloading, ready, installing, or that
	// failed.
	Version string `json:"version,omitempty"`
	// Done and Total are the package's bytes while it downloads (Total 0
	// when the server did not say).
	Done  int64 `json:"done,omitempty"`
	Total int64 `json:"total,omitempty"`
	// Error is why the last download or install failed.
	Error string `json:"error,omitempty"`
	// Last is what the update that ran before this start did.
	Last *Result `json:"last,omitempty"`
}

// Result is what an update run (Apply) did, for the next start.
type Result struct {
	From  string `json:"from"`
	To    string `json:"to"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// On: the proxy was on, so the next start turns it on again.
	On bool `json:"on,omitempty"`
}

// pending is what the copy of the core running Apply is to do.
type pending struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Package is the folder of the unpacked zip.
	Package string `json:"package"`
	AppDir  string `json:"app_dir"`
	// Args are the app's arguments for its next start.
	Args []string `json:"args,omitempty"`
	// Wait are the processes to wait for: the app and the core.
	Wait []int `json:"wait"`
	On   bool  `json:"on,omitempty"`
}

// errNotSupported: updates install by themselves on these systems only
// (platformSupported); elsewhere the app offers the download.
var errNotSupported = errors.New("updates install by themselves on Windows, Linux, and macOS only")

// errUpdateRunning: another update that uses the same data folder runs;
// it starts the app again when it is done.
var errUpdateRunning = errors.New("another update of this app is running")

// AppArgs are the app's arguments that an update passes on to the next
// start: started with the computer, or hidden in the tray. A copy started
// with its own data folder also keeps "--data-dir=<that folder>" (Install).
var AppArgs = []string{"--autostart", "--hidden"}

// InstallRequest is the app's request to install a downloaded version.
type InstallRequest struct {
	Version string
	// PID is the app's process, which quits once the request returns.
	PID int
	// Args are the app's arguments for its next start: AppArgs, and
	// "--data-dir=" naming this core's data folder.
	Args []string
	// On: the proxy is on, so the next start turns it on again.
	On bool
}

// Options configure a Manager.
type Options struct {
	// DataDir is the app's data folder (user-data); the update keeps its
	// files in its "update" folder.
	DataDir string
	// AppDir is the app's folder (AppDir): the app and core/, or the
	// macOS app's bundle.
	AppDir string
	// Current is this version (version.Current).
	Current string
	// Transports are the ways to download, in order of preference
	// (Daemon.UpdateTransports): through the proxy while it is on, then
	// directly. A loopback ServerEnv server is reached directly.
	Transports func() []http.RoundTripper
}

// Manager downloads and installs updates for one copy of the app.
type Manager struct {
	opts    Options
	dir     string
	baseURL string
	direct  bool

	mu     sync.Mutex
	status Status
}

// Supported reports whether this core can update the app it belongs to: on
// Windows, Linux, or macOS, with an update key built in, a version number,
// the app beside it, and a data folder where the update's renames work
// (dataDirOK: on Windows the app's own user-data, elsewhere one on the
// app's drive, with an app the user may change). It returns "" when it can,
// else why not.
func Supported(appDir, dataDir, current string) string {
	switch {
	case !platformSupported:
		return errNotSupported.Error()
	case !signing.Available():
		return signing.ErrNoKey.Error()
	case current == "" || current == "0.0.0":
		return "a development build has no version to update from"
	}
	if why := dataDirOK(appDir, dataDir); why != "" {
		return why
	}
	if _, err := os.Stat(filepath.Join(appDir, filepath.FromSlash(thePackage.app))); err != nil {
		return "the app is not beside this core"
	}
	return ""
}

// New opens the update folder of a copy of the app: it reads what the
// update before this start did, removes what that update left, and finds a
// newer version downloaded before (StateReady). While an update runs for
// the same app (another core of an older start), it leaves everything as
// it is.
func New(opts Options) *Manager {
	m := &Manager{opts: opts, dir: filepath.Join(opts.DataDir, "update"), baseURL: releasesURL, status: Status{State: StateIdle}}
	if server := strings.TrimRight(os.Getenv(ServerEnv), "/"); server != "" {
		m.baseURL = server + "/releases/download/"
		if u, err := url.Parse(server); err == nil {
			if ip := net.ParseIP(u.Hostname()); u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()) {
				m.direct = true
			}
		}
		logx.Info("update: releases from server=%s", server)
	}
	if updating(opts.DataDir) {
		logx.Warn("update: another update of this app is running; its files stay")
		return m
	}
	m.status.Last = m.takeResult()
	m.clean()
	m.findReady()
	return m
}

// Status returns the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	if s.Last != nil {
		last := *s.Last
		s.Last = &last
	}
	return s
}

func (m *Manager) setStatus(f func(*Status)) {
	m.mu.Lock()
	f(&m.status)
	m.mu.Unlock()
}

func (m *Manager) versionDir(v string) string { return filepath.Join(m.dir, v) }
func (m *Manager) newDir() string             { return filepath.Join(m.dir, "new") }
func (m *Manager) backupDir() string          { return filepath.Join(m.dir, "backup") }
func (m *Manager) updaterDir() string         { return filepath.Join(m.dir, "updater") }
func (m *Manager) pendingFile() string        { return filepath.Join(m.dir, "pending.json") }
func (m *Manager) resultFile() string         { return filepath.Join(m.dir, "result.json") }

// takeResult reads and removes the last update's result.
func (m *Manager) takeResult() *Result {
	data, err := os.ReadFile(m.resultFile())
	if err != nil {
		return nil
	}
	os.Remove(m.resultFile())
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		logx.Warn("update: unreadable result err=%s", logx.Err(err))
		return nil
	}
	if r.OK {
		logx.Info("update: installed from=%s to=%s", r.From, r.To)
	} else {
		logx.Warn("update: did not install from=%s to=%s err=%s", r.From, r.To, r.Error)
	}
	return &r
}

// clean removes what an earlier update left: the unpacked package (also an
// unpacking that stopped halfway), the old files, its instructions, and the
// copy of the core (which may still be exiting, so it is tried again for a
// while).
func (m *Manager) clean() {
	for _, p := range []string{m.newDir(), m.newDir() + ".part", m.backupDir(), m.pendingFile()} {
		if err := os.RemoveAll(p); err != nil {
			logx.Warn("update: removing %s failed err=%s", p, logx.Err(err))
		}
	}
	if _, err := os.Stat(m.updaterDir()); err == nil {
		go func() {
			for i := 0; i < 30; i++ {
				if os.RemoveAll(m.updaterDir()) == nil {
					return
				}
				time.Sleep(time.Second)
			}
			logx.Warn("update: the copy of the core in %s could not be removed", m.updaterDir())
		}()
	}
}

// findReady keeps the newest downloaded version that is newer than this
// one and still checks out, and removes the others.
func (m *Manager) findReady() {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return
	}
	var newer []string
	for _, e := range entries {
		if !e.IsDir() || !isVersion(e.Name()) {
			continue
		}
		if version.Newer(e.Name(), m.opts.Current) {
			newer = append(newer, e.Name())
		} else {
			os.RemoveAll(m.versionDir(e.Name()))
		}
	}
	slices.SortFunc(newer, func(a, b string) int {
		if version.Newer(a, b) {
			return -1
		}
		return 1
	})
	for i, v := range newer {
		if i == 0 {
			if _, err := m.checkPackage(v); err == nil {
				m.status = Status{State: StateReady, Version: v, Last: m.status.Last}
				logx.Info("update: version=%s is downloaded", v)
				continue
			} else {
				logx.Warn("update: the downloaded version=%s does not check out err=%s", v, logx.Err(err))
			}
		}
		os.RemoveAll(m.versionDir(v))
	}
}

// isVersion reports whether name looks like "major.minor.patch".
func isVersion(name string) bool {
	parts := strings.Split(name, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// badRelease is a release that does not check out: trying another way to
// download it would not help.
type badRelease struct{ err error }

func (e *badRelease) Error() string { return e.err.Error() }
func (e *badRelease) Unwrap() error { return e.err }

func bad(format string, a ...any) error { return &badRelease{fmt.Errorf(format, a...)} }

// Download starts downloading version v in the background, unless it is
// downloading or downloaded already. v must be newer than this version.
func (m *Manager) Download(v string) error {
	if !isVersion(v) {
		return fmt.Errorf("%q is not a version", v)
	}
	if !version.Newer(v, m.opts.Current) {
		return fmt.Errorf("version %s is not newer than this one (%s)", v, m.opts.Current)
	}
	m.mu.Lock()
	switch m.status.State {
	case StateDownloading, StateInstalling:
		busy := m.status.Version
		m.mu.Unlock()
		if busy == v {
			return nil
		}
		return fmt.Errorf("version %s is %s", busy, m.status.State)
	case StateReady:
		if m.status.Version == v {
			m.mu.Unlock()
			return nil
		}
	}
	m.status = Status{State: StateDownloading, Version: v, Last: m.status.Last}
	m.mu.Unlock()
	go m.download(v)
	return nil
}

// download fetches version v the first way that works; a release that does
// not check out is not tried another way.
func (m *Manager) download(v string) {
	logx.Info("update: downloading version=%s", v)
	var err error
	for _, t := range m.transports() {
		if err = m.fetch(t, v); err == nil {
			break
		}
		logx.Warn("update: downloading version=%s failed err=%s", v, logx.Err(err))
		var b *badRelease
		if errors.As(err, &b) {
			break
		}
	}
	if err != nil {
		os.RemoveAll(m.versionDir(v))
		m.setStatus(func(s *Status) { *s = Status{State: StateFailed, Version: v, Error: err.Error(), Last: s.Last} })
		return
	}
	// Only the newest download is kept.
	if entries, err := os.ReadDir(m.dir); err == nil {
		for _, e := range entries {
			if e.IsDir() && isVersion(e.Name()) && e.Name() != v {
				os.RemoveAll(m.versionDir(e.Name()))
			}
		}
	}
	logx.Info("update: version=%s downloaded", v)
	m.setStatus(func(s *Status) {
		*s = Status{State: StateReady, Version: v, Done: s.Done, Total: s.Total, Last: s.Last}
	})
}

func (m *Manager) transports() []http.RoundTripper {
	if m.direct || m.opts.Transports == nil {
		return []http.RoundTripper{&http.Transport{Proxy: nil}}
	}
	return m.opts.Transports()
}

// fetch downloads the signed list and the package of version v through t,
// and keeps them in the version's folder once they check out.
func (m *Manager) fetch(t http.RoundTripper, v string) error {
	base := m.baseURL + "v" + v + "/"
	small := &http.Client{Transport: t, Timeout: listTimeout}
	defer small.CloseIdleConnections()
	sums, err := get(small, base+SumsFile, maxSumsSize)
	if err != nil {
		return err
	}
	sig, err := get(small, base+SigFile, maxSigSize)
	if err != nil {
		return err
	}
	name := AssetName(v)
	want, err := checkedHash(sums, sig, name)
	if err != nil {
		return err
	}
	dir := m.versionDir(v)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := filepath.Join(dir, name+".part")
	defer os.Remove(part)
	big := &http.Client{Transport: t, Timeout: packageTimeout}
	defer big.CloseIdleConnections()
	if err := m.getPackage(big, base+name, part, want); err != nil {
		return err
	}
	for file, data := range map[string][]byte{SumsFile: sums, SigFile: sig} {
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
			return err
		}
	}
	return os.Rename(part, filepath.Join(dir, name))
}

// get reads a small file.
func get(client *http.Client, u string, limit int64) ([]byte, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, bad("the release has no %s", fileOf(u))
		}
		return nil, fmt.Errorf("%s: HTTP %d", fileOf(u), resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, bad("%s is too large", fileOf(u))
	}
	return data, nil
}

// fileOf is the file name at the end of URL u.
func fileOf(u string) string { return u[strings.LastIndex(u, "/")+1:] }

// getPackage downloads the package into file, counting its bytes in the
// status, and checks its SHA-256 against want.
func (m *Manager) getPackage(client *http.Client, u, file, want string) error {
	// A download that receives nothing for stallTimeout ends: a tunnel or
	// a network can stop without closing the connection.
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	stall := time.AfterFunc(stallTimeout, func() { cancel(errStalled) })
	defer stall.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return stalled(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return bad("the release has no %s", fileOf(u))
		}
		return fmt.Errorf("%s: HTTP %d", fileOf(u), resp.StatusCode)
	}
	if resp.ContentLength > maxPackageSize {
		return bad("%s is too large", fileOf(u))
	}
	m.setStatus(func(s *Status) { s.Done, s.Total = 0, max(resp.ContentLength, 0) })
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h, progress{m, stall}), io.LimitReader(resp.Body, maxPackageSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return stalled(ctx, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return bad("%s does not match the release's checksum", fileOf(u))
	}
	return nil
}

// stallTimeout is how long a download may receive nothing; tests shorten
// it.
var stallTimeout = time.Minute

var errStalled = errors.New("the download stopped receiving data")

// stalled is errStalled when ctx ended for it, else err.
func stalled(ctx context.Context, err error) error {
	if errors.Is(context.Cause(ctx), errStalled) {
		return errStalled
	}
	return err
}

// progress counts downloaded bytes in the status, and puts off the stall
// timer.
type progress struct {
	m     *Manager
	stall *time.Timer
}

func (p progress) Write(b []byte) (int, error) {
	p.stall.Reset(stallTimeout)
	p.m.setStatus(func(s *Status) { s.Done += int64(len(b)) })
	return len(b), nil
}

// checkedHash checks the list's signature and returns the SHA-256 that it
// gives name.
func checkedHash(sums, sig []byte, name string) (string, error) {
	if err := signing.Verify(sums, sig); err != nil {
		return "", bad("the release's %s: %w", SumsFile, err)
	}
	hash, ok := ParseSums(sums)[name]
	if !ok {
		return "", bad("the release's %s does not list %s", SumsFile, name)
	}
	return hash, nil
}

// ParseSums reads sha256sum's output: "<hex>  <name>" lines ("*<name>" in
// binary mode).
func ParseSums(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		hash, name, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || len(hash) != sha256.Size*2 {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*")
		if _, err := hex.DecodeString(hash); err == nil && name != "" {
			out[name] = strings.ToLower(hash)
		}
	}
	return out
}

// checkPackage checks a downloaded version again, as Install does just
// before it installs, and returns its package.
func (m *Manager) checkPackage(v string) (string, error) {
	dir := m.versionDir(v)
	sums, err := os.ReadFile(filepath.Join(dir, SumsFile))
	if err != nil {
		return "", err
	}
	sig, err := os.ReadFile(filepath.Join(dir, SigFile))
	if err != nil {
		return "", err
	}
	name := AssetName(v)
	want, err := checkedHash(sums, sig, name)
	if err != nil {
		return "", err
	}
	pkg := filepath.Join(dir, name)
	f, err := os.Open(pkg)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return "", fmt.Errorf("%s changed since it was downloaded", name)
	}
	return pkg, nil
}

// Install installs the downloaded version req.Version: it checks the
// package again, unpacks it, and starts a copy of this core with
// -update, which waits until the app (req.PID) and this core have exited.
// The app quits once it returns; it must not return an error for the app
// to quit.
func (m *Manager) Install(req InstallRequest) error {
	for _, a := range req.Args {
		if !slices.Contains(AppArgs, a) && !m.ownDataDir(a) {
			return fmt.Errorf("the app's argument %q is not passed on", a)
		}
	}
	if req.PID <= 0 {
		return errors.New("the app's process id is missing")
	}
	m.mu.Lock()
	if m.status.State != StateReady || m.status.Version != req.Version {
		m.mu.Unlock()
		return fmt.Errorf("version %s is not downloaded", req.Version)
	}
	m.status.State = StateInstalling
	m.mu.Unlock()
	if err := m.prepare(req); err != nil {
		logx.Error("update: installing version=%s failed err=%s", req.Version, logx.Err(err))
		m.setStatus(func(s *Status) {
			*s = Status{State: StateFailed, Version: req.Version, Error: err.Error(), Last: s.Last}
		})
		return err
	}
	logx.Info("update: installing version=%s; waiting for the app to quit", req.Version)
	return nil
}

// ownDataDir reports whether the app's argument a is "--data-dir=" naming
// this core's data folder: a copy started with it starts again with it.
func (m *Manager) ownDataDir(a string) bool {
	dir, ok := strings.CutPrefix(a, "--data-dir=")
	if !ok || dir == "" {
		return false
	}
	have, err1 := filepath.Abs(dir)
	want, err2 := filepath.Abs(m.opts.DataDir)
	return err1 == nil && err2 == nil && have == want
}

// prepare readies everything the copy of the core needs, and starts it.
func (m *Manager) prepare(req InstallRequest) error {
	pkg, err := m.checkPackage(req.Version)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(m.newDir()); err != nil {
		return err
	}
	if err := thePackage.unpack(pkg, m.newDir()); err != nil {
		os.RemoveAll(m.newDir())
		return fmt.Errorf("unpacking %s: %w", filepath.Base(pkg), err)
	}
	for _, f := range []string{thePackage.app, thePackage.core} {
		if _, err := os.Stat(filepath.Join(m.newDir(), filepath.FromSlash(f))); err != nil {
			os.RemoveAll(m.newDir())
			return bad("%s has no %s", filepath.Base(pkg), f)
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(m.updaterDir()); err != nil {
		return fmt.Errorf("removing the previous update's copy of the core: %w", err)
	}
	updater := filepath.Join(m.updaterDir(), filepath.Base(self))
	if err := copyFile(self, updater); err != nil {
		return fmt.Errorf("copying the core: %w", err)
	}
	p := pending{
		From: m.opts.Current, To: req.Version, Package: m.newDir(), AppDir: m.opts.AppDir,
		Args: req.Args, Wait: []int{req.PID, os.Getpid()}, On: req.On,
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.pendingFile(), data, 0o644); err != nil {
		return err
	}
	if err := launchUpdater(updater, m.opts.DataDir); err != nil {
		os.Remove(m.pendingFile())
		return fmt.Errorf("starting the copy of the core: %w", err)
	}
	return nil
}

// copyFile copies the executable at from to a new executable at to.
func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
