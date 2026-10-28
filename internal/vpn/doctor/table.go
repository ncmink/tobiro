package doctor

import (
	"strings"
)

// InetRoute is one IPv4 forwarding-table entry.
type InetRoute struct {
	Destination string
	Gateway     string
	Netif       string
}

// ParseNetstatInet reads the Internet section of `netstat -rn -f inet`.
func ParseNetstatInet(raw string) []InetRoute {
	lines := strings.Split(raw, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "Internet:" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}

	var routes []InetRoute
	for _, line := range lines[start:] {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "Internet6") || strings.HasPrefix(trim, "Routing tables") {
			break
		}
		fields := strings.Fields(trim)
		if len(fields) < 4 {
			continue
		}
		if strings.EqualFold(fields[0], "Destination") {
			continue
		}
		netif := fields[len(fields)-1]
		if isNumeric(netif) && len(fields) >= 5 {
			netif = fields[len(fields)-2]
		}
		routes = append(routes, InetRoute{
			Destination: fields[0],
			Gateway:     fields[1],
			Netif:       netif,
		})
	}
	return routes
}

// CapturesAllTraffic reports whether the table sends every IPv4 packet into
// the tunnel: a default route via the tunnel, or the 0/1 + 128.0/1 pair.
func CapturesAllTraffic(routes []InetRoute, tunnelIface, assignedIP string) bool {
	if tunnelIface == "" && assignedIP == "" {
		return false
	}
	var defaultVia, lowHalf, highHalf bool
	for _, route := range routes {
		if !viaTunnel(route, tunnelIface, assignedIP) {
			continue
		}
		switch normalizeDest(route.Destination) {
		case "default", "0/0", "0.0.0.0/0", "0.0.0.0":
			defaultVia = true
		case "0/1", "0.0.0.0/1":
			lowHalf = true
		case "128.0/1", "128.0.0.0/1":
			highHalf = true
		}
	}
	return defaultVia || (lowHalf && highHalf)
}

func viaTunnel(route InetRoute, tunnelIface, assignedIP string) bool {
	if tunnelIface != "" && route.Netif == tunnelIface {
		return true
	}
	return assignedIP != "" && route.Gateway == assignedIP
}

func normalizeDest(dest string) string {
	return strings.ToLower(strings.TrimSpace(dest))
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
