// Package nc reads and drives the VPN services macOS itself manages, through
// the documented `scutil --nc` interface.
//
// Two kinds of service show up there, and the distinction matters:
//
//	PPP:L2TP, IPSec       configured in System Settings, owned by macOS
//	VPN:<bundle id>       installed by a third-party app's NetworkExtension
//
// macOS can start and stop the first kind directly. For the second it only
// reports state, since the vendor app owns the session — which is still the
// most reliable state available for those clients, and is attributed to a vendor
// by the bundle identifier.
//
// Parsing lives here rather than in an adapter so the two callers share one
// tested implementation.
package nc

import (
	"regexp"
	"strings"
)

// Service is one entry from `scutil --nc list`.
type Service struct {
	// ID is the service UUID, the handle every other command takes.
	ID string

	// Name is the label shown in System Settings, e.g. "VPN CAT".
	Name string

	// State is the word macOS printed: Connected, Disconnected, Connecting…
	State string

	// Kind is the bracketed type, e.g. "PPP:L2TP", "IPSec" or
	// "VPN:com.surfshark.vpnclient.macos.direct".
	Kind string

	Connected bool
}

// SystemManaged reports whether macOS owns this service and can therefore start
// and stop it. Third-party NetworkExtension services are driven by their own
// app, and appear as "VPN:<bundle id>".
func (s Service) SystemManaged() bool {
	return s.Kind != "" && !strings.HasPrefix(s.Kind, "VPN:")
}

// ProviderID is the bundle identifier of the app owning a NetworkExtension
// service, or "" for a service macOS manages itself.
func (s Service) ProviderID() string {
	id, found := strings.CutPrefix(s.Kind, "VPN:")
	if !found {
		return ""
	}
	return id
}

// Reading is the state of one service from `scutil --nc status`.
type Reading struct {
	Connected bool

	// State is the word macOS printed on the first line.
	State string

	// AssignedIP and Interface are only set when macOS publishes an IPv4 block
	// for the service. Several NetworkExtension providers publish none.
	AssignedIP string
	Interface  string
}

// listLine splits `* (Connected) <UUID> PPP --> L2TP  "VPN CAT"  [PPP:L2TP]`.
var listLine = regexp.MustCompile(`^\*?\s*\((\w+)\)\s+(\S+)\s+(.*)$`)

// kindBracket captures the trailing "[...]" type marker.
var kindBracket = regexp.MustCompile(`\[([^\]]+)\]\s*$`)

// ParseList reads the service table printed by `scutil --nc list`.
func ParseList(raw string) []Service {
	var out []Service
	for _, line := range strings.Split(raw, "\n") {
		match := listLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if match == nil {
			continue
		}
		rest := match[3]

		service := Service{
			ID:        match[2],
			State:     match[1],
			Connected: strings.EqualFold(match[1], "Connected"),
			Name:      quotedName(rest),
		}
		if kind := kindBracket.FindStringSubmatch(rest); kind != nil {
			service.Kind = strings.TrimSpace(kind[1])
		}
		out = append(out, service)
	}
	return out
}

// quotedName pulls the human-readable service name out of the trailing columns,
// falling back to the raw text when macOS prints no quoted name.
func quotedName(rest string) string {
	if start := strings.Index(rest, `"`); start >= 0 {
		if end := strings.Index(rest[start+1:], `"`); end >= 0 {
			return rest[start+1 : start+1+end]
		}
	}
	return strings.TrimSpace(rest)
}

// ParseStatus reads `scutil --nc status <service>`:
//
//	Connected
//	Extended Status <dictionary> {
//	  IPv4 : <dictionary> {
//	    Addresses : <array> {
//	      0 : 192.168.255.159
//	    }
//	    DestAddresses : <array> {
//	      0 : 192.168.255.1
//	    }
//	    InterfaceName : ppp0
func ParseStatus(raw string) Reading {
	lines := strings.Split(raw, "\n")

	var reading Reading
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			reading.State = trimmed
			reading.Connected = strings.EqualFold(trimmed, "Connected")
			break
		}
	}

	inAddresses := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// "DestAddresses" is the peer, not this machine, so the block header is
		// matched exactly rather than by substring.
		switch {
		case trimmed == "Addresses : <array> {":
			inAddresses = true
			continue
		case inAddresses && trimmed == "}":
			inAddresses = false
			continue
		}

		if inAddresses {
			if reading.AssignedIP == "" {
				if _, value, found := strings.Cut(trimmed, " : "); found {
					reading.AssignedIP = strings.TrimSpace(value)
				}
			}
			continue
		}
		if name, found := strings.CutPrefix(trimmed, "InterfaceName : "); found && reading.Interface == "" {
			reading.Interface = strings.TrimSpace(name)
		}
	}
	return reading
}

// FindByName resolves a service by its System Settings name, case-insensitively
// so a slightly differently typed name still matches.
func FindByName(services []Service, name string) (Service, bool) {
	wanted := strings.TrimSpace(name)
	if wanted == "" {
		return Service{}, false
	}
	for _, service := range services {
		if strings.EqualFold(service.Name, wanted) {
			return service, true
		}
	}
	return Service{}, false
}

// SystemManagedNames lists the services macOS manages, for error messages that
// tell the user what they could have picked instead.
func SystemManagedNames(services []Service) []string {
	var out []string
	for _, service := range services {
		if service.SystemManaged() {
			out = append(out, service.Name)
		}
	}
	return out
}
