// Command sign-release signs Tobiro release artifacts with the project's
// Ed25519 updater key, and exports a matching public key in the form the
// Wails updater is able to pin.
//
// The signature covers the SHA-256 digest of an artifact rather than its bytes,
// because that is what the framework's "ed25519" scheme verifies: the updater
// hashes the download as it streams it and checks the signature against that
// digest. Signing the bytes directly would produce a signature that never
// verifies.
//
// Prefer `wails3 updater genkey` and `wails3 updater sign` when the CLI is
// available; this tool matches biebie-kube CI (UPDATER_SIGNING_KEY in GitHub).
//
// Usage:
//
//	sign-release Tobiro-v0.0.1-darwin-universal.zip   # writes <name>.sig
//	sign-release -public updater.key.pub > updater-key.pem
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/crypto/ssh"
)

const keyEnvVar = "UPDATER_SIGNING_KEY"

const defaultKeyFile = "updater.key"

func main() {
	log.SetFlags(0)
	log.SetPrefix("sign-release: ")

	keyFile := flag.String("key", "", "private key file (default $"+keyEnvVar+", else ./"+defaultKeyFile+")")
	publicKey := flag.String("public", "", "convert this public key (OpenSSH line or PKIX PEM) to PKIX PEM on stdout and exit")
	flag.Usage = usage
	flag.Parse()

	if *publicKey != "" {
		if err := exportPublicKey(*publicKey, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}

	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}

	key, err := loadPrivateKey(*keyFile)
	if err != nil {
		log.Fatal(err)
	}

	for _, artifact := range flag.Args() {
		sigPath, err := signArtifact(key, artifact)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "signed %s -> %s\n", artifact, sigPath)
	}
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `usage:
  sign-release [-key FILE] ARTIFACT...   sign each artifact, writing ARTIFACT.sig
  sign-release -public FILE              export a public key as PKIX PEM

flags:
`)
	flag.PrintDefaults()
}

func signArtifact(key ed25519.PrivateKey, artifact string) (string, error) {
	digest, err := fileDigest(artifact)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(key, digest)
	encoded := base64.StdEncoding.EncodeToString(signature) + "\n"

	sigPath := artifact + ".sig"
	if err := os.WriteFile(sigPath, []byte(encoded), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", sigPath, err)
	}
	return sigPath, nil
}

func fileDigest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return hasher.Sum(nil), nil
}

func loadPrivateKey(file string) (ed25519.PrivateKey, error) {
	if file != "" {
		return readPrivateKeyFile(file)
	}
	if material := os.Getenv(keyEnvVar); material != "" {
		key, err := parsePrivateKey([]byte(material))
		if err != nil {
			return nil, fmt.Errorf("$%s: %w", keyEnvVar, err)
		}
		return key, nil
	}
	return readPrivateKeyFile(defaultKeyFile)
}

func readPrivateKeyFile(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading signing key: %w", err)
	}
	key, err := parsePrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}

func parsePrivateKey(raw []byte) (ed25519.PrivateKey, error) {
	if len(raw) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(raw), nil
	}

	if block, _ := pem.Decode(raw); block != nil {
		if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			switch k := key.(type) {
			case ed25519.PrivateKey:
				return k, nil
			case *ed25519.PrivateKey:
				return *k, nil
			}
		}
		if key, err := ssh.ParseRawPrivateKey(raw); err == nil {
			return ed25519FromSSH(key)
		}
	}

	parsed, err := ssh.ParseRawPrivateKey(raw)
	if err != nil {
		var needsPassphrase *ssh.PassphraseMissingError
		if errors.As(err, &needsPassphrase) {
			return nil, errors.New("key is passphrase-protected; re-export it without one for unattended signing")
		}
		return nil, fmt.Errorf("not a usable private key: %w", err)
	}
	return ed25519FromSSH(parsed)
}

func ed25519FromSSH(parsed any) (ed25519.PrivateKey, error) {
	switch key := parsed.(type) {
	case ed25519.PrivateKey:
		return key, nil
	case *ed25519.PrivateKey:
		return *key, nil
	default:
		return nil, fmt.Errorf("key is %T, but the updater requires Ed25519", parsed)
	}
}

func exportPublicKey(path string, out io.Writer) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading public key: %w", err)
	}

	if block, _ := pem.Decode(raw); block != nil && block.Type == "PUBLIC KEY" {
		_, err := out.Write(raw)
		return err
	}

	parsed, _, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return fmt.Errorf("%s: not an OpenSSH or PKIX public key: %w", path, err)
	}

	crypto, ok := parsed.(ssh.CryptoPublicKey)
	if !ok {
		return fmt.Errorf("%s: unsupported key type %q", path, parsed.Type())
	}
	edPublic, ok := crypto.CryptoPublicKey().(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("%s: key is %q, but the updater requires Ed25519", path, parsed.Type())
	}

	der, err := x509.MarshalPKIXPublicKey(edPublic)
	if err != nil {
		return fmt.Errorf("encoding public key: %w", err)
	}
	return pem.Encode(out, &pem.Block{Type: "PUBLIC KEY", Bytes: der})
}
