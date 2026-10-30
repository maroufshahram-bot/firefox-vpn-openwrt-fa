package tunservice

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/armanlife/firefox-vpn-client/core/internal/signing"
)

// Updates of the service must be signed with the key built into it
// (signing.Key): the release key for a release build, whose private half
// only CI holds; a developer key for a local build. Without one the
// service takes no updates and is replaced by installing it again, as an
// administrator. A signature is required because the service runs with
// the system's rights and any user who may talk to it could otherwise make
// it run their program.

// maxUpdateSize bounds the executable an update reads.
const maxUpdateSize = 64 << 20

// SignatureFile is where the signature of an executable is: beside it, but
// in a macOS app (Contents/MacOS) in its Contents/Resources, since only
// code may be in Contents/MacOS of a signed app.
func SignatureFile(exe string) string {
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		return filepath.Join(filepath.Dir(dir), "Resources", filepath.Base(exe)+".sig")
	}
	return exe + ".sig"
}

// readSigned reads the executable at path, and checks it against its
// signature file and the built-in key. The bytes it returns are the ones
// checked: the file may change afterwards, the copy may not.
func readSigned(path string) ([]byte, error) {
	if !signing.Available() {
		return nil, errors.New("this build of the service takes no updates; install the new one instead")
	}
	data, err := readLimited(path, maxUpdateSize)
	if err != nil {
		return nil, err
	}
	sig, err := readLimited(SignatureFile(path), 1024)
	if err != nil {
		return nil, fmt.Errorf("the update is not signed: %w", err)
	}
	if err := signing.Verify(data, sig); err != nil {
		return nil, fmt.Errorf("checking the update: %w", err)
	}
	return data, nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is too large", path)
	}
	return data, nil
}
