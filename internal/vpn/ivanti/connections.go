package ivanti

import (
	"regexp"
	"strings"

	"tobiro/internal/domain"
)

// savedConnection is one entry the Ivanti client keeps in connstore.dat. Each
// entry carries its own saved account, so two entries may share a gateway.
type savedConnection struct {
	ID   string
	Name string
	URI  string
}

var (
	connstoreHeader = regexp.MustCompile(`^ive\s+"([^"]+)"\s*\{\s*$`)
	connstoreField  = regexp.MustCompile(`^([a-z-]+):\s*"(.*)"\s*$`)
	connectionEvent = regexp.MustCompile(`\[Connection\] Connection (establishing|terminating|terminated): '(.+?)'`)
)

// parseConnstore reads the `ive` blocks from connstore.dat.
func parseConnstore(raw string) []savedConnection {
	var out []savedConnection
	var current *savedConnection
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if current == nil {
			if m := connstoreHeader.FindStringSubmatch(line); m != nil {
				current = &savedConnection{ID: m[1]}
			}
			continue
		}
		if line == "}" {
			if current.Name != "" {
				out = append(out, *current)
			}
			current = nil
			continue
		}
		if m := connstoreField.FindStringSubmatch(line); m != nil {
			switch m[1] {
			case "friendly-name":
				current.Name = m[2]
			case "uri":
				current.URI = m[2]
			}
		}
	}
	return out
}

// activeConnectionName replays ISACEvents.log and returns the connection that
// was last established and not yet torn down. The client runs one connection
// at a time, so at most one name is returned.
func activeConnectionName(events string) string {
	active := ""
	for _, line := range strings.Split(events, "\n") {
		m := connectionEvent.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		switch m[1] {
		case "establishing":
			active = m[2]
		case "terminated":
			if m[2] == active {
				active = ""
			}
		}
	}
	return active
}

// connectionOptions offers saved Ivanti connections to the connection form.
func connectionOptions(saved []savedConnection, active string) []domain.VendorProfileOption {
	out := make([]domain.VendorProfileOption, 0, len(saved))
	for _, c := range saved {
		out = append(out, domain.VendorProfileOption{
			Name:      c.Name,
			Detail:    c.URI,
			Connected: active != "" && strings.EqualFold(c.Name, active),
		})
	}
	return out
}

// scopeToConnection narrows a gateway-level status to the Ivanti connection a
// card is bound to. Cards without one, or a reading with no known active
// connection, keep the gateway-level answer.
func scopeToConnection(status *domain.VPNStatus, vendorProfile, active string) *domain.VPNStatus {
	want := strings.TrimSpace(vendorProfile)
	if status == nil || want == "" || active == "" || status.State != domain.StateConnected {
		return status
	}
	if strings.EqualFold(want, active) {
		return status
	}
	return &domain.VPNStatus{
		State:  domain.StateDisconnected,
		Known:  true,
		Detail: "Ivanti is connected with “" + active + "”, not “" + want + "”.",
	}
}
