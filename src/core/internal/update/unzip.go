package update

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unzip unpacks the zip at file into dir. The release's zip holds one
// folder ("Firefox VPN/...") whose contents go into dir; names that would
// land outside dir are refused, on every system as on Windows (no colon:
// a drive, or an NTFS stream), and so are links.
func unzip(file, dir string) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	top := topFolder(r.File)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		name := strings.TrimPrefix(zipName(f), top)
		if name == "" {
			continue
		}
		if strings.Contains(name, ":") || !filepath.IsLocal(filepath.FromSlash(strings.TrimSuffix(name, "/"))) {
			return fmt.Errorf("%q is outside the app's folder", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%q is a link", f.Name)
		}
		dest := filepath.Join(dir, filepath.FromSlash(name))
		if f.FileInfo().IsDir() || strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := unzipFile(f, dest); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// zipName is an entry's name with forward slashes.
func zipName(f *zip.File) string {
	return strings.TrimPrefix(strings.ReplaceAll(f.Name, `\`, "/"), "./")
}

// topFolder is "<folder>/" when every entry is inside that one folder, else
// "".
func topFolder(files []*zip.File) string {
	top := ""
	for _, f := range files {
		first, _, inside := strings.Cut(zipName(f), "/")
		if !inside && !f.FileInfo().IsDir() {
			return ""
		}
		if top == "" {
			top = first
		} else if first != top {
			return ""
		}
	}
	if top == "" {
		return ""
	}
	return top + "/"
}

func unzipFile(f *zip.File, dest string) error {
	if f.UncompressedSize64 > 4*maxPackageSize {
		return errors.New("too large")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
