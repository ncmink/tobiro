// Package sshtunnel reaches a customer network through SSH local port
// forwarding, for the case where the firewall opens port 22 and nothing else.
//
// It is a peer of internal/vpn rather than a provider inside it. A VPN adapter
// orchestrates software someone else installed; this package speaks the
// protocol itself, in this process, using golang.org/x/crypto/ssh. There is no
// child `ssh` to lose track of, no window to leave open, and the local ports it
// lends are closed the moment the tunnel is.
//
// Nothing here writes a secret anywhere. A key passphrase or password arrives
// on the credential for the duration of one connect call and is released with it.
package sshtunnel

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"tobiro/internal/domain"
)

// dialTimeout bounds a single connection attempt. A firewall that drops packets
// silently is the normal failure here, and it looks exactly like a hang.
const dialTimeout = 20 * time.Second

// clientConfig turns a profile and one credential into everything the dialler
// needs, or explains what the account is missing.
func clientConfig(profile domain.VPNProfile, credential domain.Credential, hostKey ssh.HostKeyCallback) (*ssh.ClientConfig, error) {
	user := strings.TrimSpace(profile.SSH.User)
	if user == "" {
		return nil, errors.New("this connection has no login name for the SSH server")
	}

	methods, err := authMethods(credential)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: hostKey,
		Timeout:         dialTimeout,
		ClientVersion:   "SSH-2.0-BiebieAccess",
	}, nil
}

// authMethods maps an account onto the ways it can prove itself.
//
// The account says which one it is rather than the code trying each in turn:
// an SSH server counts every failed attempt against MaxAuthTries, so offering
// three methods to a server that wanted one is how an account with a perfectly
// good key gets locked out of its own tunnel.
func authMethods(credential domain.Credential) ([]ssh.AuthMethod, error) {
	switch credential.AuthMethod {
	case domain.AuthSSHKey:
		signer, err := loadPrivateKey(credential)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case domain.AuthPassword:
		if credential.Secret == "" {
			return nil, fmt.Errorf("no password saved for %q", credential.DisplayName())
		}
		password := credential.Secret
		return []ssh.AuthMethod{
			ssh.Password(password),
			// Many sshd configurations present password auth as
			// keyboard-interactive instead. Answering every prompt with the
			// password is what the ssh command line does here too.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = password
				}
				return answers, nil
			}),
		}, nil

	case domain.AuthSSHAgent, "":
		signers, err := agentSigners()
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil

	default:
		return nil, fmt.Errorf("%q cannot be used to authenticate an SSH tunnel", credential.AuthMethod)
	}
}

// agentSigners reads the keys the running ssh-agent holds.
//
// The signers are resolved now rather than through ssh.PublicKeysCallback so an
// agent that is not running is reported as such, instead of surfacing later as
// an unexplained "no supported methods remain".
func agentSigners() ([]ssh.Signer, error) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, errors.New("no ssh-agent is running for this session (SSH_AUTH_SOCK is not set)")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("connect to the ssh-agent: %w", err)
	}

	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ask the ssh-agent for its keys: %w", err)
	}
	if len(signers) == 0 {
		conn.Close()
		return nil, errors.New("the ssh-agent holds no keys — add one with ssh-add")
	}
	// The connection stays open: a signer is a handle on the agent, and closing
	// it here would make every signature fail. It is released when the process
	// exits, which is the same lifetime the ssh command line gives it.
	return signers, nil
}

// loadPrivateKey reads a key file from disk. Its contents are parsed and
// discarded; only the signer is kept, and the passphrase never leaves this call.
func loadPrivateKey(credential domain.Credential) (ssh.Signer, error) {
	path, err := expandHome(credential.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("no private key file is set on %q", credential.DisplayName())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("private key %s does not exist", path)
		}
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}

	if credential.Secret != "" {
		signer, err := ssh.ParsePrivateKeyWithPassphrase(raw, []byte(credential.Secret))
		if err != nil {
			return nil, fmt.Errorf("unlock private key %s: %w", path, err)
		}
		return signer, nil
	}

	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil, fmt.Errorf(
				"private key %s has a passphrase — edit the account and save it as the password", path)
		}
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}
	return signer, nil
}

// expandHome resolves the tilde every SSH key path is written with.
func expandHome(path string) (string, error) {
	p := strings.TrimSpace(path)
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	return filepath.Join(home, p[2:]), nil
}
