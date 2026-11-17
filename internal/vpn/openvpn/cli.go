package openvpn

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"tobiro/internal/sysexec"
)

const cliTimeout = 15 * time.Second

// Profile is one entry returned by OpenVPN Connect --list-profiles.
type Profile struct {
	ID       string
	Name     string
	Host     string
	Username string
	Type     string
}

type cliProfile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Host            string `json:"host"`
	Username        string `json:"username"`
	Type            string `json:"type"`
	ServerOverride  string `json:"server-override"`
	ProfileName     string `json:"profileName"`
}

// listProfiles runs the documented --list-profiles command and parses the
// output. OpenVPN's help sample uses unquoted keys; newer builds may print JSON.
func listProfiles(ctx context.Context, cliPath string) ([]Profile, error) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	res, err := sysexec.Run(ctx, cliPath, "--list-profiles")
	if err != nil {
		return nil, fmt.Errorf("OpenVPN Connect --list-profiles: %w", err)
	}
	out := strings.TrimSpace(res.Stdout)
	if out == "" {
		return nil, nil
	}
	if strings.Contains(strings.ToLower(out), "accept gdpr") {
		return nil, fmt.Errorf("accept GDPR in OpenVPN Connect before Tobiro can list profiles")
	}

	raw, err := parseListProfilesOutput(out)
	if err != nil {
		return nil, err
	}
	outProfiles := make([]Profile, 0, len(raw))
	for _, p := range raw {
		name := strings.TrimSpace(firstNonEmpty(p.Name, p.ProfileName))
		host := strings.TrimSpace(p.Host)
		if host == "" || strings.EqualFold(host, "none") {
			host = strings.TrimSpace(p.ServerOverride)
		}
		outProfiles = append(outProfiles, Profile{
			ID:       strings.TrimSpace(p.ID),
			Name:     name,
			Host:     host,
			Username: strings.TrimSpace(p.Username),
			Type:     strings.TrimSpace(p.Type),
		})
	}
	return outProfiles, nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func parseListProfilesOutput(out string) ([]cliProfile, error) {
	var profiles []cliProfile
	if err := json.Unmarshal([]byte(out), &profiles); err == nil {
		return profiles, nil
	}
	coerced := quoteUnquotedJSONKeys(out)
	coerced = quoteUnquotedJSONStringValues(coerced)
	if err := json.Unmarshal([]byte(coerced), &profiles); err == nil {
		return profiles, nil
	}
	if loose, ok := parseListProfilesLoose(out); ok {
		return loose, nil
	}
	return nil, fmt.Errorf("parse --list-profiles output: not JSON (%d bytes)", len(out))
}

var unquotedKey = regexp.MustCompile(`([\{\[,]\s*)([a-zA-Z][a-zA-Z0-9-]*)(\s*:)`)

func quoteUnquotedJSONKeys(s string) string {
	return unquotedKey.ReplaceAllString(s, `$1"$2"$3`)
}

var looseField = regexp.MustCompile(`(?i)^\s*([a-z][a-z0-9-]*)\s*:\s*(.+?)\s*,?\s*$`)

// parseListProfilesLoose reads the help-doc pseudo-array (unquoted keys and
// values on separate lines inside braces).
func parseListProfilesLoose(out string) ([]cliProfile, bool) {
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "[") {
		return nil, false
	}
	var blocks []string
	depth := 0
	start := -1
	for i, r := range out {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				blocks = append(blocks, out[start:i+1])
				start = -1
			}
		}
	}
	if len(blocks) == 0 {
		return nil, false
	}
	var profiles []cliProfile
	for _, block := range blocks {
		p := cliProfile{}
		for _, line := range strings.Split(block, "\n") {
			m := looseField.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(m[1]))
			val := strings.Trim(strings.TrimSpace(m[2]), `",`)
			switch key {
			case "id":
				p.ID = val
			case "name":
				p.Name = val
			case "host":
				p.Host = val
			case "username":
				p.Username = val
			case "type":
				p.Type = val
			case "server-override":
				p.ServerOverride = val
			}
		}
		if p.Name != "" || p.ID != "" {
			profiles = append(profiles, p)
		}
	}
	if len(profiles) == 0 {
		return nil, false
	}
	return profiles, true
}

var unquotedValue = regexp.MustCompile(`:\s*([a-zA-Z@][a-zA-Z0-9@._-]*)(\s*[,}\]])`)

func quoteUnquotedJSONStringValues(s string) string {
	return unquotedValue.ReplaceAllString(s, `: "$1"$2`)
}