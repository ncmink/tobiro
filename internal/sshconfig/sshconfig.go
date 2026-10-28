// Package sshconfig reads OpenSSH client configuration files.
package sshconfig

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"tobiro/internal/domain"
)

const defaultSSHPort = 22

// Host is one named Host block from an OpenSSH config file.
type Host struct {
	Alias        string
	HostName     string
	User         string
	Port         int
	IdentityFile string
	Forwards     []domain.PortForward
}

type block struct {
	aliases      []string
	hostName     string
	user         string
	port         int
	identityFile string
	forwards     []domain.PortForward
}

// Parse reads OpenSSH config text into discrete Host entries.
//
// Include is not expanded here; use ParseFile to follow Include. Match and
// ProxyJump are not expanded. Host patterns containing *, ?, or ! are skipped,
// as is Host *.
func Parse(raw []byte) []Host {
	var out []Host
	var cur *block

	emit := func(b *block) {
		if b == nil || len(b.aliases) == 0 {
			return
		}
		for _, alias := range b.aliases {
			if skipAlias(alias) {
				continue
			}
			out = append(out, finalize(alias, *b))
		}
	}

	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		trimmed := strings.TrimSpace(stripComment(string(line)))
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 1 {
			continue
		}
		key := strings.ToLower(fields[0])
		value := ""
		if len(fields) > 1 {
			value = strings.Join(fields[1:], " ")
		}

		switch key {
		case "host":
			emit(cur)
			cur = &block{aliases: append([]string(nil), fields[1:]...)}
		case "hostname":
			if cur != nil {
				cur.hostName = value
			}
		case "user":
			if cur != nil {
				cur.user = value
			}
		case "port":
			if cur != nil {
				if p, err := strconv.Atoi(value); err == nil {
					cur.port = p
				}
			}
		case "identityfile":
			if cur != nil {
				cur.identityFile = unquote(value)
			}
		case "localforward":
			if cur != nil {
				f, err := parseLocalForward(fields[1:])
				if err == nil {
					cur.forwards = append(cur.forwards, f)
				}
			}
		}
	}
	emit(cur)
	return out
}

func skipAlias(alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" || alias == "*" {
		return true
	}
	for _, r := range alias {
		if r == '*' || r == '?' || r == '!' {
			return true
		}
	}
	return false
}

func finalize(alias string, b block) Host {
	hostName := strings.TrimSpace(b.hostName)
	if hostName == "" {
		hostName = strings.TrimSpace(alias)
	}
	port := b.port
	if port == 0 {
		port = defaultSSHPort
	}
	forwards := make([]domain.PortForward, len(b.forwards))
	copy(forwards, b.forwards)
	return Host{
		Alias:        strings.TrimSpace(alias),
		HostName:     hostName,
		User:         strings.TrimSpace(b.user),
		Port:         port,
		IdentityFile: strings.TrimSpace(b.identityFile),
		Forwards:     forwards,
	}
}

func stripComment(line string) string {
	inQuote := false
	for i, r := range line {
		switch r {
		case '"', '\'':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return line[:i]
			}
		}
	}
	return line
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseLocalForward(args []string) (domain.PortForward, error) {
	if len(args) == 0 {
		return domain.PortForward{}, fmt.Errorf("local forward is empty")
	}
	if len(args) == 3 {
		local, err := strconv.Atoi(args[0])
		if err != nil {
			return domain.PortForward{}, fmt.Errorf("local port %q is not a number", args[0])
		}
		remote, err := strconv.Atoi(args[2])
		if err != nil {
			return domain.PortForward{}, fmt.Errorf("remote port %q is not a number", args[2])
		}
		f := domain.PortForward{
			Name:       domain.SuggestForwardName(remote),
			LocalPort:  local,
			RemoteHost: strings.TrimSpace(args[1]),
			RemotePort: remote,
		}
		if f.RemoteHost == "" {
			f.RemoteHost = "localhost"
		}
		if err := f.Validate(); err != nil {
			return domain.PortForward{}, err
		}
		return f, nil
	}
	spec := strings.Join(args, " ")
	if len(args) == 2 && !strings.Contains(args[0], ":") {
		spec = args[0] + ":" + args[1]
	}
	return parseForwardColonSpec(spec)
}

func parseForwardColonSpec(spec string) (domain.PortForward, error) {
	colonParts := strings.Split(spec, ":")
	if len(colonParts) == 4 {
		bind := strings.ToLower(colonParts[0])
		switch bind {
		case "localhost", "127.0.0.1", "::1", "":
		default:
			return domain.PortForward{}, fmt.Errorf(
				"forward %q listens on %s; Tobiro only forwards to %s", spec, colonParts[0], domain.LoopbackAddress)
		}
		colonParts = colonParts[1:]
	}
	if len(colonParts) != 3 {
		return domain.PortForward{}, fmt.Errorf("forward %q is not in the form localPort:host:remotePort", spec)
	}
	local, err := strconv.Atoi(colonParts[0])
	if err != nil {
		return domain.PortForward{}, fmt.Errorf("forward %q: local port %q is not a number", spec, colonParts[0])
	}
	remote, err := strconv.Atoi(colonParts[2])
	if err != nil {
		return domain.PortForward{}, fmt.Errorf("forward %q: remote port %q is not a number", spec, colonParts[2])
	}
	f := domain.PortForward{
		Name:       domain.SuggestForwardName(remote),
		LocalPort:  local,
		RemoteHost: strings.TrimSpace(colonParts[1]),
		RemotePort: remote,
	}
	if f.RemoteHost == "" {
		f.RemoteHost = "localhost"
	}
	if err := f.Validate(); err != nil {
		return domain.PortForward{}, err
	}
	return f, nil
}
