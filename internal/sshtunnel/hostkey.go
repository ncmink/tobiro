package sshtunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"tobiro/internal/domain"
	"tobiro/internal/logging"
)

// verifier decides whether the server that answered is the one we meant to
// reach.
//
// The whole point of an SSH tunnel here is that it carries Kubernetes
// credentials into a customer network, so this is the security boundary of the
// feature. There is deliberately no way to turn it off: ssh.InsecureIgnoreHostKey
// appears nowhere in this package, and no profile field can reach it.
type verifier struct {
	// path is the known_hosts file this machine already keeps for the ssh
	// command line. Reusing it rather than inventing a private trust store
	// means a server the engineer has already accepted in a terminal is
	// accepted here without asking again.
	path string

	profileID string
	prompter  Prompter
	log       *logging.Logger

	// ctx is the connect call's context, so a prompt cannot outlive the attempt
	// that raised it.
	ctx context.Context

	mu sync.Mutex
}

func newVerifier(ctx context.Context, profileID, knownHosts string, prompter Prompter, log *logging.Logger) *verifier {
	if knownHosts == "" {
		knownHosts = defaultKnownHostsPath()
	}
	return &verifier{
		path:      knownHosts,
		profileID: profileID,
		prompter:  prompter,
		log:       log,
		ctx:       ctx,
	}
}

func defaultKnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// callback is the ssh.HostKeyCallback handed to the dialler.
func (v *verifier) callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	check, err := v.load()
	if err != nil {
		return err
	}

	if check != nil {
		err = check(hostname, remote, key)
		if err == nil {
			return nil
		}

		// Anything that is not a plain "no entry for this host" — a revoked
		// key, an unreadable file — is a refusal, not a question.
		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			// The host is known and answered with a different key. This is
			// either a rebuilt server or someone standing in the middle, and
			// nothing available here can tell those apart — so it is refused
			// rather than turned into a prompt the user learns to click through.
			return failf(FailHostKeyChanged,
				"the host key for %s has changed since it was last accepted (%s). "+
					"If the server really was rebuilt, remove its line from %s and try again",
				hostname, ssh.FingerprintSHA256(key), v.path)
		}
	}

	return v.accept(hostname, key)
}

// load builds the check from known_hosts, treating a missing file as "nothing
// has been accepted yet" rather than as a failure.
func (v *verifier) load() (ssh.HostKeyCallback, error) {
	if v.path == "" {
		return nil, nil
	}
	check, err := knownhosts.New(v.path)
	if err == nil {
		return check, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return nil, fmt.Errorf("read %s: %w", v.path, err)
}

// accept asks the user about a host nobody on this machine has met, and records
// the answer so they are only asked once.
func (v *verifier) accept(hostname string, key ssh.PublicKey) error {
	if v.prompter == nil {
		return fmt.Errorf(
			"%s is not in %s and there is no window to confirm it in", hostname, v.path)
	}

	confirmed, err := v.prompter.ConfirmHostKey(v.ctx, domain.HostKeyRequest{
		ProfileID:   v.profileID,
		Host:        hostname,
		Algorithm:   key.Type(),
		Fingerprint: ssh.FingerprintSHA256(key),
		KnownHosts:  v.path,
	})
	if err != nil {
		return err
	}
	if !confirmed {
		return failf(FailHostKeyUnknown, "the host key for %s was not accepted", hostname)
	}

	if err := v.remember(hostname, key); err != nil {
		// Failing to write the file is not a reason to refuse a key the user
		// just vouched for; it only means they will be asked again next time.
		v.log.Warn("Accepted the host key for %s but could not record it in %s: %v", hostname, v.path, err)
	} else {
		v.log.Info("Recorded the host key for %s in %s", hostname, v.path)
	}
	return nil
}

func (v *verifier) remember(hostname string, key ssh.PublicKey) error {
	if v.path == "" {
		return errors.New("no known_hosts location for this user")
	}
	if err := os.MkdirAll(filepath.Dir(v.path), 0o700); err != nil {
		return err
	}
	// Read/write rather than append-only: the trailing-newline check below has
	// to look at the last byte already in the file.
	file, err := os.OpenFile(v.path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()

	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	if err := ensureTrailingNewline(file); err != nil {
		return err
	}
	_, err = file.WriteString(line)
	return err
}

// ensureTrailingNewline stops a new entry being glued onto a final line that
// was written without one, which would silently corrupt both.
func ensureTrailingNewline(file *os.File) error {
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, info.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	_, err = file.WriteString("\n")
	return err
}
