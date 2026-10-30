package update

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// linuxPackage is the Linux app's tar.gz for this processor
// (scripts/build-linux.sh): a firefox-vpn folder with the app, its lib/ and
// data/, and core/.
var linuxPackage = appPackage{
	name:   func(v string) string { return "Firefox-VPN-" + v + "-Linux-" + runtime.GOARCH + ".tar.gz" },
	app:    "firefox-vpn",
	core:   "core/firefox-vpn-core",
	unpack: untar,
}

// macPackage is the macOS app's tar.gz (scripts/build-macos.sh): Firefox
// VPN.app, for Apple silicon and Intel, as the disk image holds it. The
// app's folder is the bundle, so an update replaces its Contents.
var macPackage = appPackage{
	name:   func(v string) string { return "Firefox-VPN-" + v + "-macOS.tar.gz" },
	app:    "Contents/MacOS/Firefox VPN",
	core:   "Contents/MacOS/firefox-vpn-core",
	unpack: untar,
}

// untar unpacks the tar.gz at file into dir, as unzip does a zip: one top
// folder's contents go into dir, and names that would land outside it are
// refused, as are hard links and symbolic links that point outside it (a
// macOS bundle's frameworks link within themselves); a file keeps whether
// it is executable.
func untar(file, dir string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	type entry struct {
		name string
		hdr  *tar.Header
		// staged is where a file's content waits.
		staged string
	}
	// The top folder is known once every name is read, so the entries
	// are read first; the files go to a temporary folder beside dir.
	var entries []entry
	stage := dir + ".part"
	if err := os.RemoveAll(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	r := tar.NewReader(gz)
	for i := 0; ; i++ {
		hdr, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "" || name == "." {
			continue
		}
		if !filepath.IsLocal(filepath.FromSlash(strings.TrimSuffix(name, "/"))) {
			return fmt.Errorf("%q is outside the app's folder", hdr.Name)
		}
		e := entry{name: name, hdr: hdr}
		switch hdr.Typeflag {
		case tar.TypeDir, tar.TypeSymlink:
		case tar.TypeReg:
			if hdr.Size > 4*maxPackageSize {
				return fmt.Errorf("%s: too large", name)
			}
			e.staged = filepath.Join(stage, fmt.Sprint(i))
			if err := writeFile(r, e.staged, hdr.FileInfo().Mode()); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		case tar.TypeLink:
			return fmt.Errorf("%q is a hard link", hdr.Name)
		default:
			continue
		}
		entries = append(entries, e)
	}
	top := ""
	for _, e := range entries {
		first, _, inside := strings.Cut(e.name, "/")
		if !inside && e.hdr.Typeflag != tar.TypeDir {
			top = ""
			break
		}
		if top == "" {
			top = first
		} else if first != top {
			top = ""
			break
		}
	}
	if top != "" {
		top += "/"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	type link struct{ name, dest, target string }
	var links []link
	// The archive's links by lower-case name (APFS ignores case), to
	// follow them as the file system will.
	linked := map[string]string{}
	for _, e := range entries {
		// The top folder's own entry; the app inside it has its name too
		// (firefox-vpn/firefox-vpn).
		if top != "" && strings.TrimSuffix(e.name, "/")+"/" == top {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(e.name, top), "/")
		if name == "" {
			continue
		}
		// "<top>/../x" was local before the top folder was cut.
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("%q is outside the app's folder", e.hdr.Name)
		}
		dest := filepath.Join(dir, filepath.FromSlash(name))
		switch e.hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		case tar.TypeSymlink:
			links = append(links, link{name, dest, e.hdr.Linkname})
			linked[strings.ToLower(name)] = e.hdr.Linkname
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.Rename(e.staged, dest); err != nil {
			return err
		}
	}
	// The links come last, so that no file is written through one; each
	// must lie and lead within the app's folder, also through the others.
	for _, l := range links {
		if l.target == "" || path.IsAbs(l.target) ||
			!within(path.Dir(l.name), linked) || !within(path.Dir(l.name)+"/"+l.target, linked) {
			return fmt.Errorf("%q links outside the app's folder", l.name)
		}
		if err := os.MkdirAll(filepath.Dir(l.dest), 0o755); err != nil {
			return err
		}
		if err := os.Symlink(filepath.FromSlash(l.target), l.dest); err != nil {
			return err
		}
	}
	return nil
}

// within reports whether the slash path p, relative to the app's folder,
// stays inside it when the archive's links (linked) are followed as the
// file system follows them: ".." after a link counts from where the link
// leads. An absolute target, a loop, or too many links count as outside.
func within(p string, linked map[string]string) bool {
	parts := strings.Split(p, "/")
	cur := ""
	for hops := 0; len(parts) > 0; {
		c := parts[0]
		parts = parts[1:]
		switch c {
		case "", ".":
			continue
		case "..":
			if cur == "" {
				return false
			}
			if cur = path.Dir(cur); cur == "." {
				cur = ""
			}
			continue
		}
		next := path.Join(cur, c)
		target, ok := linked[strings.ToLower(next)]
		if !ok {
			cur = next
			continue
		}
		if hops++; hops > 40 || target == "" || path.IsAbs(target) {
			return false
		}
		// The target counts from the link's folder, cur.
		parts = append(strings.Split(target, "/"), parts...)
	}
	return true
}

// writeFile copies r into a new file at dest: executable (0755) when mode
// says so, else 0644; on the disk before the swap renames it into place.
func writeFile(r io.Reader, dest string, mode os.FileMode) error {
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
