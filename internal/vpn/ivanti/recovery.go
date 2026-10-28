package ivanti

import (
	"net"
	"strconv"
	"strings"
)

// recoveredRoute is one forwarding entry from RMrecovery-VPN.dat.
//
// Each line is: 2,0,<destination>,<prefix>,<next hop>,...
type recoveredRoute struct {
	Destination net.IP
	Prefix      int
	NextHop     net.IP
}

func parseRecovery(raw string) []recoveredRoute {
	var routes []recoveredRoute
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 5 {
			continue
		}
		dest := net.ParseIP(strings.TrimSpace(fields[2]))
		next := net.ParseIP(strings.TrimSpace(fields[4]))
		if dest == nil {
			continue
		}
		prefix, err := strconv.Atoi(strings.TrimSpace(fields[3]))
		if err != nil || prefix < 0 || prefix > 128 {
			continue
		}
		routes = append(routes, recoveredRoute{
			Destination: dest,
			Prefix:      prefix,
			NextHop:     next,
		})
	}
	return routes
}

// recoveryCapturesAll is true when the client installed a full-tunnel recipe:
// default (0.0.0.0/0) or the 0.0.0.0/1 + 128.0.0.0/1 pair that together cover
// every IPv4 address while remaining more specific than the LAN default.
func recoveryCapturesAll(routes []recoveredRoute) bool {
	var lowHalf, highHalf bool
	for _, route := range routes {
		ip := route.Destination.To4()
		if ip == nil {
			if route.Destination.IsUnspecified() && route.Prefix == 0 {
				return true
			}
			continue
		}
		if ip.Equal(net.IPv4zero) && route.Prefix == 0 {
			return true
		}
		if ip.Equal(net.IPv4zero) && route.Prefix == 1 {
			lowHalf = true
		}
		if ip[0] == 128 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 && route.Prefix == 1 {
			highHalf = true
		}
	}
	return lowHalf && highHalf
}

func nextHopsOf(routes []recoveredRoute) map[string]struct{} {
	out := make(map[string]struct{})
	for _, route := range routes {
		if route.NextHop == nil || route.NextHop.IsUnspecified() {
			continue
		}
		out[route.NextHop.String()] = struct{}{}
	}
	return out
}

func gatewaysOf(routes []recoveredRoute) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, route := range routes {
		ip := route.Destination.To4()
		if ip == nil || route.Prefix != 32 {
			continue
		}
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
