package openvpn

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Session is the last CONNECTED event in the log that was not followed by
// DISCONNECTED in the replay window.
type Session struct {
	Username   string
	Host       string
	Port       string
	Resolved   string
	Interface  string
	AssignedIP string
}

var (
	connectedLine = regexp.MustCompile(
		`EVENT: CONNECTED ([^@\s]+)@([^:\s]+):(\d+)\s+\(([^)]+)\).*?\s+on\s+([^/]+)/([^/\s]+)/`,
	)
	disconnectedLine = regexp.MustCompile(`EVENT: DISCONNECTED`)
)

// replayLog walks the log in order and returns the tunnel that would still be up.
func replayLog(log string) (Session, bool) {
	var current Session
	active := false
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if disconnectedLine.MatchString(line) {
			active = false
			current = Session{}
			continue
		}
		if m := connectedLine.FindStringSubmatch(line); m != nil {
			current = Session{
				Username:   m[1],
				Host:       m[2],
				Port:       m[3],
				Resolved:   m[4],
				Interface:  m[5],
				AssignedIP: m[6],
			}
			active = true
		}
	}
	return current, active
}

// live reports whether assignedIP is bound to a local non-loopback interface.
func live(assignedIP string) (iface string, ok bool) {
	assignedIP = strings.TrimSpace(assignedIP)
	if assignedIP == "" || net.ParseIP(assignedIP) == nil {
		return "", false
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	for _, candidate := range ifaces {
		if candidate.Flags&net.FlagUp == 0 || candidate.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := candidate.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			netIP, valid := addr.(*net.IPNet)
			if !valid {
				continue
			}
			if netIP.IP.String() == assignedIP {
				return candidate.Name, true
			}
		}
	}
	return "", false
}

// matchesTarget reports whether the live session belongs to the profile
// identified by host and username from --list-profiles.
func (s Session) matchesTarget(host, username string) bool {
	host = strings.TrimSpace(host)
	username = strings.TrimSpace(username)
	if host == "" {
		return false
	}
	hosts := []string{s.Host, s.Resolved}
	for _, h := range hosts {
		if hostEqual(host, h) {
			if username == "" || strings.EqualFold(username, s.Username) {
				return true
			}
		}
	}
	return false
}

func hostEqual(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	ia := net.ParseIP(a)
	ib := net.ParseIP(b)
	return ia != nil && ib != nil && ia.Equal(ib)
}

func sessionDetail(s Session) string {
	return fmt.Sprintf("Tunnel up on %s with address %s.", s.Interface, s.AssignedIP)
}