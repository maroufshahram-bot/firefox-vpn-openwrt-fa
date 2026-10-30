// Command sign makes and uses the Ed25519 key that updates are signed with
// (internal/signing): a release's SHA256SUMS.txt, which the Windows app
// checks before it installs a new version (internal/update), and the TUN
// service's executable, which the installed service checks before it
// replaces itself (internal/tunservice):
//
//	go run ./tools/sign -keygen FILE      write a new private key to FILE, print its public key
//	go run ./tools/sign -public           print the public key (for -ldflags -X ...signing.Key=)
//	go run ./tools/sign FILE...           write FILE.sig beside each file
//
// The private key comes from -key FILE, else from FIREFOX_VPN_SIGNING_KEY
// (base64, as CI holds it in a secret), else from the file named by
// FIREFOX_VPN_SIGNING_KEY_FILE. A key file holds the base64 seed on one
// line; keep it out of the repository (*.signing-key is ignored).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	keygen := flag.String("keygen", "", "write a new private key to this file")
	keyFile := flag.String("key", "", "the private key file")
	public := flag.Bool("public", false, "print the public key")
	flag.Parse()
	if err := run(*keygen, *keyFile, *public, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "sign:", err)
		os.Exit(1)
	}
}

func run(keygen, keyFile string, public bool, files []string) error {
	if keygen != "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		if err := os.WriteFile(keygen, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
			return err
		}
		fmt.Println(publicKey(priv))
		return nil
	}
	priv, err := loadKey(keyFile)
	if err != nil {
		return err
	}
	if public {
		fmt.Println(publicKey(priv))
		return nil
	}
	if len(files) == 0 {
		return errors.New("name the files to sign")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))
		if err := os.WriteFile(f+".sig", []byte(sig+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func publicKey(priv ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
}

func loadKey(file string) (ed25519.PrivateKey, error) {
	text := os.Getenv("FIREFOX_VPN_SIGNING_KEY")
	if file == "" && text == "" {
		file = os.Getenv("FIREFOX_VPN_SIGNING_KEY_FILE")
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		text = string(data)
	}
	if text == "" {
		return nil, errors.New("no key: use -key, FIREFOX_VPN_SIGNING_KEY, or FIREFOX_VPN_SIGNING_KEY_FILE")
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the key is not a base64 Ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
