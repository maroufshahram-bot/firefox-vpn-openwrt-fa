package update

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/armanlife/firefox-vpn-client/core/internal/signing"
)

// The tests make the Windows zip on every system; untar_test.go tries the
// Linux and macOS archives. (The init of process_linux.go or
// process_darwin.go runs first: its file sorts first.)
func init() { thePackage = windowsPackage }

// release is a test release: its files by name, served under
// /releases/download/v<version>/.
type release struct {
	version string
	files   map[string][]byte
	priv    ed25519.PrivateKey
}

// newRelease makes a signed release of version v with its zip and a Setup
// (which updates never use), and makes the build's key the release's.
func newRelease(t *testing.T, v string) *release {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	old := signing.Key
	signing.Key = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { signing.Key = old })
	r := &release{version: v, priv: priv, files: map[string][]byte{
		AssetName(v): appZip(t, map[string]string{
			"Firefox VPN/firefox-vpn.exe":              "app " + v,
			"Firefox VPN/flutter_windows.dll":          "flutter " + v,
			"Firefox VPN/data/app.so":                  "assets " + v,
			"Firefox VPN/core/firefox-vpn-core.exe":    "core " + v,
			"Firefox VPN/core/firefox-vpn-tun.exe":     "tun " + v,
			"Firefox VPN/core/firefox-vpn-tun.exe.sig": "sig " + v,
		}),
		"Firefox-VPN-" + v + "-Windows-Setup-x64.exe": []byte("MZ setup " + v),
	}}
	r.sign()
	return r
}

// sign writes the release's SHA256SUMS.txt and its signature.
func (r *release) sign() {
	var sums strings.Builder
	for name, data := range r.files {
		if name == SumsFile || name == SigFile {
			continue
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	r.files[SumsFile] = []byte(sums.String())
	r.files[SigFile] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(r.priv, r.files[SumsFile])) + "\n")
}

// serve serves the release and points ServerEnv at it.
func (r *release) serve(t *testing.T) *httptest.Server {
	t.Helper()
	prefix := "/releases/download/v" + r.version + "/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		data, ok := r.files[strings.TrimPrefix(req.URL.Path, prefix)]
		if !ok || !strings.HasPrefix(req.URL.Path, prefix) {
			http.NotFound(w, req)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(ServerEnv, srv.URL)
	return srv
}

func appZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(content))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// appFolder makes a portable copy of version v with its data.
func appFolder(t *testing.T, v string, installed bool) (appDir, dataDir string) {
	t.Helper()
	appDir = t.TempDir()
	for name, content := range map[string]string{
		"firefox-vpn.exe":           "app " + v,
		"flutter_windows.dll":       "flutter " + v,
		"data/app.so":               "assets " + v,
		"data/old-asset.png":        "gone in the new version",
		"core/firefox-vpn-core.exe": "core " + v,
		"user-data/config.json":     "the user's settings",
		"notes.txt":                 "the user's own file",
	} {
		write(t, filepath.Join(appDir, filepath.FromSlash(name)), content)
	}
	if installed {
		write(t, filepath.Join(appDir, "unins000.exe"), "uninstaller")
	}
	return appDir, filepath.Join(appDir, "user-data")
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newManager(appDir, dataDir string) *Manager {
	return New(Options{DataDir: dataDir, AppDir: appDir, Current: "1.0.0"})
}

// waitState waits until the manager leaves the downloading state.
func waitState(t *testing.T, m *Manager) Status {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s := m.Status()
		if s.State != StateDownloading {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("still downloading: %+v", s)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestParseSumsReadsSha256sumOutput(t *testing.T) {
	a, b := strings.Repeat("ab", 32), strings.Repeat("CD", 32)
	got := ParseSums([]byte(a + "  Firefox-VPN-1.2.3-Windows-x64.zip\n" + b + " *SHA256SUMS.txt\r\nnot a line\n\n" + "abc  short.txt\n"))
	if len(got) != 2 || got["Firefox-VPN-1.2.3-Windows-x64.zip"] != a || got["SHA256SUMS.txt"] != strings.ToLower(b) {
		t.Fatalf("got %v", got)
	}
}

func TestPortableAndInstalledCopiesDownloadTheZip(t *testing.T) {
	for _, installed := range []bool{false, true} {
		r := newRelease(t, "1.2.3")
		r.serve(t)
		appDir, dataDir := appFolder(t, "1.0.0", installed)
		m := newManager(appDir, dataDir)
		if err := m.Download("1.2.3"); err != nil {
			t.Fatal(err)
		}
		s := waitState(t, m)
		if s.State != StateReady || s.Version != "1.2.3" || s.Error != "" {
			t.Fatalf("installed=%v: %+v", installed, s)
		}
		name := AssetName("1.2.3")
		if got := read(t, filepath.Join(dataDir, "update", "1.2.3", name)); got != string(r.files[name]) {
			t.Fatalf("installed=%v: the package is %q", installed, got)
		}
		if s.Done != int64(len(r.files[name])) {
			t.Fatalf("installed=%v: %d of %d bytes counted", installed, s.Done, len(r.files[name]))
		}
		// Asking again changes nothing; a new start finds it downloaded.
		if err := m.Download("1.2.3"); err != nil || m.Status().State != StateReady {
			t.Fatalf("again: %v %+v", err, m.Status())
		}
		if s := newManager(appDir, dataDir).Status(); s.State != StateReady || s.Version != "1.2.3" {
			t.Fatalf("installed=%v: after a restart %+v", installed, s)
		}
	}
}

func TestAReleaseThatDoesNotCheckOutIsNotKept(t *testing.T) {
	for name, spoil := range map[string]func(r *release){
		"another key's signature": func(r *release) {
			_, other, _ := ed25519.GenerateKey(nil)
			r.files[SigFile] = []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(other, r.files[SumsFile])))
		},
		"a changed list": func(r *release) {
			r.files[SumsFile] = append(r.files[SumsFile], "00  extra\n"...)
		},
		"a changed package": func(r *release) {
			r.files[AssetName("1.2.3")] = []byte("something else")
		},
		"no signature": func(r *release) { delete(r.files, SigFile) },
		"no package":   func(r *release) { delete(r.files, AssetName("1.2.3")) },
	} {
		r := newRelease(t, "1.2.3")
		spoil(r)
		r.serve(t)
		appDir, dataDir := appFolder(t, "1.0.0", false)
		m := newManager(appDir, dataDir)
		if err := m.Download("1.2.3"); err != nil {
			t.Fatal(err)
		}
		s := waitState(t, m)
		if s.State != StateFailed || s.Error == "" {
			t.Errorf("%s: %+v", name, s)
		}
		if _, err := os.Stat(filepath.Join(dataDir, "update", "1.2.3")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: the download was kept", name)
		}
	}
}

// A download that stops receiving data ends after stallTimeout instead of
// waiting for the whole download's timeout.
func TestAStalledDownloadFails(t *testing.T) {
	r := newRelease(t, "1.2.3")
	prefix := "/releases/download/v1.2.3/"
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name := strings.TrimPrefix(req.URL.Path, prefix)
		if name != AssetName("1.2.3") {
			w.Write(r.files[name])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(r.files[name])))
		w.Write(r.files[name][:10])
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	t.Setenv(ServerEnv, srv.URL)
	stallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { stallTimeout = time.Minute })
	appDir, dataDir := appFolder(t, "1.0.0", false)
	m := newManager(appDir, dataDir)
	start := time.Now()
	if err := m.Download("1.2.3"); err != nil {
		t.Fatal(err)
	}
	s := waitState(t, m)
	if s.State != StateFailed || !strings.Contains(s.Error, "stopped receiving") {
		t.Fatalf("%+v", s)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
	if _, err := os.Stat(filepath.Join(dataDir, "update", "1.2.3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the partial download was kept")
	}
}

func TestOnlyANewerVersionIsDownloaded(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.0.0", false)
	m := newManager(appDir, dataDir)
	for _, v := range []string{"1.0.0", "0.9.9", "", "1.2", "../1.2.3"} {
		if err := m.Download(v); err == nil {
			t.Errorf("%q: accepted", v)
		}
	}
}

func TestADownloadChangedOnDiskIsNotInstalled(t *testing.T) {
	r := newRelease(t, "1.2.3")
	r.serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	m := newManager(appDir, dataDir)
	m.Download("1.2.3")
	waitState(t, m)
	write(t, filepath.Join(dataDir, "update", "1.2.3", AssetName("1.2.3")), "changed")
	launchUpdater = func(string, string) error { t.Fatal("the updater started"); return nil }
	t.Cleanup(func() { launchUpdater = startUpdater })
	if err := m.Install(InstallRequest{Version: "1.2.3", PID: 1}); err == nil {
		t.Fatal("installed")
	}
	if s := m.Status(); s.State != StateFailed {
		t.Fatalf("%+v", s)
	}
	// A new start drops it.
	if s := newManager(appDir, dataDir).Status(); s.State != StateIdle {
		t.Fatalf("after a restart %+v", s)
	}
}

// installed runs Install for version 1.2.3 with the updater's start
// recorded instead of done, and returns its instructions.
func installed(t *testing.T, appDir, dataDir string, args ...string) pending {
	t.Helper()
	m := newManager(appDir, dataDir)
	if err := m.Download("1.2.3"); err != nil {
		t.Fatal(err)
	}
	if s := waitState(t, m); s.State != StateReady {
		t.Fatalf("%+v", s)
	}
	var started []string
	launchUpdater = func(exe, data string) error {
		started = append(started, exe, data)
		return nil
	}
	t.Cleanup(func() { launchUpdater = startUpdater })
	if err := m.Install(InstallRequest{Version: "1.2.3", PID: 4242, Args: args, On: true}); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.State != StateInstalling {
		t.Fatalf("%+v", s)
	}
	if len(started) != 2 || started[1] != dataDir || !strings.HasPrefix(started[0], filepath.Join(dataDir, "update", "updater")) {
		t.Fatalf("started %v", started)
	}
	info, err := os.Stat(started[0])
	if err != nil {
		t.Fatalf("the copy of the core: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the copy of the core is not executable: %v", info.Mode())
	}
	var p pending
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dataDir, "update", "pending.json"))), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInstallUnpacksTheZipAndStartsACopyOfTheCore(t *testing.T) {
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	p := installed(t, appDir, dataDir, "--hidden")
	if p.From != "1.0.0" || p.To != "1.2.3" || p.AppDir != appDir || !p.On ||
		len(p.Wait) != 2 || p.Wait[0] != 4242 || p.Wait[1] != os.Getpid() || len(p.Args) != 1 || p.Args[0] != "--hidden" {
		t.Fatalf("%+v", p)
	}
	if p.Package != filepath.Join(dataDir, "update", "new") || read(t, filepath.Join(p.Package, "core", "firefox-vpn-core.exe")) != "core 1.2.3" {
		t.Fatalf("unpacked into %s", p.Package)
	}
}

func TestInstallPassesOnlyTheAppsOwnArguments(t *testing.T) {
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	m := newManager(appDir, dataDir)
	m.Download("1.2.3")
	waitState(t, m)
	if err := m.Install(InstallRequest{Version: "1.2.3", PID: 1, Args: []string{"--data-dir=C:\\elsewhere"}}); err == nil {
		t.Fatal("accepted")
	}
	if err := m.Install(InstallRequest{Version: "1.2.4", PID: 1}); err == nil {
		t.Fatal("installed a version that was not downloaded")
	}
}

func TestUnzipRefusesNamesOutsideTheFolder(t *testing.T) {
	for _, name := range []string{"../evil.exe", "Firefox VPN/../../evil.exe", "/abs.exe", `C:\evil.exe`, "Firefox VPN/core/firefox-vpn-core.exe:hidden"} {
		file := filepath.Join(t.TempDir(), "x.zip")
		os.WriteFile(file, appZip(t, map[string]string{name: "x", "Firefox VPN/firefox-vpn.exe": "app"}), 0o644)
		if err := unzip(file, filepath.Join(t.TempDir(), "out")); err == nil {
			t.Errorf("%q: unpacked", name)
		}
	}
}

func TestUnzipKeepsAZipWithoutOneTopFolderAsItIs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "x.zip")
	os.WriteFile(file, appZip(t, map[string]string{"firefox-vpn.exe": "app", "core/firefox-vpn-core.exe": "core"}), 0o644)
	out := filepath.Join(t.TempDir(), "out")
	if err := unzip(file, out); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(out, "core", "firefox-vpn-core.exe")) != "core" {
		t.Fatal("wrong layout")
	}
}

// newVersion lays out the new version's files in a folder, as Install
// unpacks them.
func newVersion(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"firefox-vpn.exe":           "app 1.2.3",
		"flutter_windows.dll":       "flutter 1.2.3",
		"data/app.so":               "assets 1.2.3",
		"core/firefox-vpn-core.exe": "core 1.2.3",
		"core/firefox-vpn-tun.exe":  "tun 1.2.3",
	} {
		write(t, filepath.Join(dir, filepath.FromSlash(name)), content)
	}
	return dir
}

func TestSwapReplacesTheAppsFilesAndKeepsTheRest(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.0.0", false)
	backup := filepath.Join(dataDir, "update", "backup")
	if err := swap(appDir, newVersion(t), backup); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"firefox-vpn.exe":           "app 1.2.3",
		"data/app.so":               "assets 1.2.3",
		"core/firefox-vpn-core.exe": "core 1.2.3",
		"core/firefox-vpn-tun.exe":  "tun 1.2.3",
		"user-data/config.json":     "the user's settings",
		"notes.txt":                 "the user's own file",
	} {
		if got := read(t, filepath.Join(appDir, filepath.FromSlash(name))); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	// The data folder is replaced whole: an asset the new version dropped
	// is gone.
	if _, err := os.Stat(filepath.Join(appDir, "data", "old-asset.png")); !errors.Is(err, os.ErrNotExist) {
		t.Error("an old asset stayed")
	}
	if read(t, filepath.Join(backup, "firefox-vpn.exe")) != "app 1.0.0" {
		t.Error("the old app is not kept aside")
	}
}

// snapshot reads every file under dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			out[filepath.ToSlash(rel)] = read(t, path)
		}
		return nil
	})
	return out
}

func TestAFailedSwapPutsTheOldVersionBack(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.0.0", false)
	before := snapshot(t, appDir)
	renameWait = 50 * time.Millisecond
	var mu sync.Mutex
	// A virus scanner holds the new core: it cannot be put in place.
	rename = func(from, to string) error {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasSuffix(from, filepath.Join("new", "core")) {
			return &os.LinkError{Op: "rename", Old: from, New: to, Err: errors.New("in use")}
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { rename, renameWait = os.Rename, 10*time.Second })
	newDir := filepath.Join(dataDir, "update", "new")
	os.MkdirAll(filepath.Dir(newDir), 0o755)
	if err := os.Rename(newVersion(t), newDir); err != nil {
		t.Skip("the test folders are on different drives")
	}
	err := swap(appDir, newDir, filepath.Join(dataDir, "update", "backup"))
	if err == nil || !strings.Contains(err.Error(), "core") {
		t.Fatalf("got %v", err)
	}
	after := snapshot(t, appDir)
	for name, content := range before {
		if after[name] != content {
			t.Errorf("%s = %q, want %q", name, after[name], content)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok && !strings.HasPrefix(name, "user-data/") {
			t.Errorf("%s was left behind", name)
		}
	}
}

func TestANewStartReadsTheResultAndCleansUp(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.2.3", false)
	dir := filepath.Join(dataDir, "update")
	for _, p := range []string{"new/firefox-vpn.exe", "backup/firefox-vpn.exe", "updater/firefox-vpn-core.exe", "1.2.3/" + SumsFile, "1.0.0/" + SumsFile, "pending.json"} {
		write(t, filepath.Join(dir, filepath.FromSlash(p)), "x")
	}
	data, _ := json.Marshal(Result{From: "1.0.0", To: "1.2.3", OK: true, On: true})
	write(t, filepath.Join(dir, "result.json"), string(data))
	m := New(Options{DataDir: dataDir, AppDir: appDir, Current: "1.2.3"})
	s := m.Status()
	if s.State != StateIdle || s.Last == nil || !s.Last.OK || !s.Last.On || s.Last.To != "1.2.3" {
		t.Fatalf("%+v", s)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		left, _ := os.ReadDir(dir)
		if len(left) == 0 {
			break
		}
		if time.Now().After(deadline) {
			var names []string
			for _, e := range left {
				names = append(names, e.Name())
			}
			t.Fatalf("left: %v", names)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s := New(Options{DataDir: dataDir, AppDir: appDir, Current: "1.2.3"}).Status(); s.Last != nil {
		t.Fatal("the result was read twice")
	}
}

func TestSupportedNeedsAKeyAVersionAndTheApp(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.0.0", false)
	if runtime.GOOS != "windows" {
		// Linux and macOS keep the data outside the app.
		dataDir = t.TempDir()
	}
	old := signing.Key
	t.Cleanup(func() { signing.Key = old })
	signing.Key = ""
	if Supported(appDir, dataDir, "1.0.0") == "" {
		t.Error("supported without a key")
	}
	signing.Key = "x"
	if Supported(appDir, dataDir, "0.0.0") == "" {
		t.Error("supported for a development build")
	}
	if Supported(t.TempDir(), dataDir, "1.0.0") == "" && platformSupported {
		t.Error("supported without the app")
	}
	// Windows wants the app's own user-data; Linux and macOS a folder on
	// the app's drive outside the app (a portable macOS copy keeps it in
	// the bundle, which the swap replaces).
	others := []string{t.TempDir()}
	if runtime.GOOS != "windows" {
		others = []string{filepath.Join(t.TempDir(), "missing"), filepath.Join(appDir, "user-data")}
	}
	for _, other := range others {
		if Supported(appDir, other, "1.0.0") == "" && platformSupported {
			t.Errorf("supported with the data folder %s", other)
		}
	}
	// A copy run as root does not update itself (Linux and macOS).
	want := platformSupported && (runtime.GOOS == "windows" || os.Geteuid() != 0)
	if got := Supported(appDir, dataDir, "1.0.0"); (got == "") != want {
		t.Errorf("got %q", got)
	}
}

// A copy started with its own data folder starts again with it after the
// update; another folder is not passed on.
func TestInstallKeepsTheDataFolderOfACopyStartedWithOne(t *testing.T) {
	newRelease(t, "1.2.3").serve(t)
	appDir, dataDir := appFolder(t, "1.0.0", false)
	p := installed(t, appDir, dataDir, "--hidden", "--data-dir="+dataDir)
	if strings.Join(p.Args, " ") != "--hidden --data-dir="+dataDir {
		t.Fatalf("args %v", p.Args)
	}
}

// A start after an unpacking that stopped halfway removes what it left.
func TestANewStartRemovesAHalfUnpackedVersion(t *testing.T) {
	appDir, dataDir := appFolder(t, "1.0.0", false)
	write(t, filepath.Join(dataDir, "update", "new.part", "3"), "half")
	newManager(appDir, dataDir)
	if _, err := os.Stat(filepath.Join(dataDir, "update", "new.part")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the half-unpacked version stayed")
	}
}
