package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SSHCommand is what an `ssh` command line means in this application's terms.
//
// The private key path is separate from the profile because it belongs to an
// account rather than to a connection: one server is routinely reached with a
// different key per engineer, which is the same reason a profile owns a
// credential list instead of a username.
type SSHCommand struct {
	Profile        ProfileInput `json:"profile"`
	PrivateKeyPath string       `json:"privateKeyPath"`
}

// sshFlagsWithValue take the next token as their argument. Only the handful
// this application understands are acted on; the rest are recognised purely so
// their argument is not mistaken for the destination.
var sshFlagsWithValue = map[byte]bool{
	'L': true, 'R': true, 'D': true, 'p': true, 'i': true, 'l': true,
	'o': true, 'b': true, 'c': true, 'e': true, 'F': true, 'I': true,
	'J': true, 'm': true, 'O': true, 'Q': true, 'S': true, 'W': true,
	'w': true, 'B': true, 'E': true, 'P': true,
}

// ParseSSHCommand turns a command someone already runs in a terminal into a
// connection they can save.
//
// This exists because the command is the artefact that actually gets passed
// around — pasted into a runbook, shared in a chat — and retyping it into six
// form fields is where the mistakes come from.
func ParseSSHCommand(raw string) (SSHCommand, error) {
	tokens := strings.Fields(strings.TrimSpace(raw))
	if len(tokens) == 0 {
		return SSHCommand{}, errors.New("paste an ssh command to read")
	}
	if strings.EqualFold(tokens[0], "ssh") {
		tokens = tokens[1:]
	}

	var (
		out         SSHCommand
		destination string
		port        int
		user        string
		forwards    []PortForward
	)

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		if !strings.HasPrefix(token, "-") || token == "-" {
			if destination != "" {
				// Everything after the destination is a command to run on the
				// server. A tunnel that also runs a command is a different
				// thing from the one being described here.
				return SSHCommand{}, fmt.Errorf(
					"this command runs %q on the server; only a plain port-forwarding command can be saved", token)
			}
			destination = token
			continue
		}

		flag := token[1]
		value := token[2:]
		if sshFlagsWithValue[flag] && value == "" {
			if i+1 >= len(tokens) {
				return SSHCommand{}, fmt.Errorf("-%c is missing its value", flag)
			}
			i++
			value = tokens[i]
		}

		switch flag {
		case 'L':
			forward, err := parseForwardSpec(value)
			if err != nil {
				return SSHCommand{}, err
			}
			forwards = append(forwards, forward)
		case 'R':
			return SSHCommand{}, errors.New("remote forwards (-R) are not supported yet")
		case 'D':
			return SSHCommand{}, errors.New("dynamic SOCKS forwards (-D) are not supported yet")
		case 'J':
			return SSHCommand{}, errors.New("jump hosts (-J) are not supported yet")
		case 'p':
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return SSHCommand{}, fmt.Errorf("port %q is not a number", value)
			}
			port = parsed
		case 'l':
			user = value
		case 'i':
			out.PrivateKeyPath = value
		}
	}

	if destination == "" {
		return SSHCommand{}, errors.New("this command names no server to connect to")
	}

	host := strings.TrimPrefix(destination, "ssh://")
	if at := strings.LastIndex(host, "@"); at >= 0 {
		user = host[:at]
		host = host[at+1:]
	}
	// A destination written as ssh://host:2222 carries the port where a bare
	// host does not, and the two forms must not disagree about which one wins.
	if colon := strings.LastIndex(host, ":"); colon >= 0 && !strings.Contains(host[colon+1:], "]") {
		if parsed, err := strconv.Atoi(host[colon+1:]); err == nil {
			host = host[:colon]
			if port == 0 {
				port = parsed
			}
		}
	}

	out.Profile = ProfileInput{
		Kind: KindSSH,
		Host: host,
		Port: port,
		SSH:  SSHSettings{User: user, Forwards: forwards},
	}.Normalise()

	if err := out.Profile.validateSSH(); err != nil {
		return SSHCommand{}, err
	}
	// A pasted command is a finished thing rather than a form halfway through
	// being typed, so it is held to the whole standard. A command with no -L in
	// it describes no tunnel at all, and reading it as an empty one would leave
	// the user looking at a form that had apparently ignored them.
	if err := out.Profile.SSH.Validate(); err != nil {
		return SSHCommand{}, err
	}
	if err := ValidatePrivateKeyPath(out.PrivateKeyPath); err != nil {
		return SSHCommand{}, err
	}
	return out, nil
}

// parseForwardSpec reads the argument of -L.
//
// OpenSSH accepts both "local:host:port" and "bind:local:host:port". The bound
// form is read only to reject a non-loopback bind address, because forwarding a
// customer's port to every machine on the local network is not something to
// carry over silently from a command someone pasted.
func parseForwardSpec(spec string) (PortForward, error) {
	parts := strings.Split(spec, ":")
	if len(parts) == 4 {
		bind := strings.ToLower(parts[0])
		switch bind {
		case "localhost", "127.0.0.1", "::1", "":
		default:
			return PortForward{}, fmt.Errorf(
				"forward %q listens on %s; Tobiro only forwards to %s", spec, parts[0], LoopbackAddress)
		}
		parts = parts[1:]
	}
	if len(parts) != 3 {
		return PortForward{}, fmt.Errorf("forward %q is not in the form localPort:host:remotePort", spec)
	}

	local, err := strconv.Atoi(parts[0])
	if err != nil {
		return PortForward{}, fmt.Errorf("forward %q: local port %q is not a number", spec, parts[0])
	}
	remote, err := strconv.Atoi(parts[2])
	if err != nil {
		return PortForward{}, fmt.Errorf("forward %q: remote port %q is not a number", spec, parts[2])
	}

	forward := PortForward{
		Name:       SuggestForwardName(remote),
		LocalPort:  local,
		RemoteHost: parts[1],
		RemotePort: remote,
	}
	if err := forward.Validate(); err != nil {
		return PortForward{}, err
	}
	return forward, nil
}

// SuggestForwardName labels the ports whose number is the whole explanation, so
// a saved tunnel reads as "Kubernetes API" rather than as four digits.
func SuggestForwardName(remotePort int) string {
	switch remotePort {
	case 6443:
		return "Kubernetes API"
	case 443:
		return "HTTPS"
	case 80:
		return "HTTP"
	case 5432:
		return "PostgreSQL"
	case 3306:
		return "MySQL"
	case 6379:
		return "Redis"
	default:
		return ""
	}
}
