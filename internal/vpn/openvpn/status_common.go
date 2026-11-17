package openvpn

import (
	"context"
	"fmt"
	"strings"

	"tobiro/internal/domain"
)

// profileByName resolves a card's saved value to one imported profile.
//
// OpenVPN Connect lists a profile as "<host> [<friendly name>]", while its
// window shows just the friendly name, so a card may hold either form, or the
// profile ID. A partial match is accepted only when exactly one profile fits.
func profileByName(profiles []Profile, name string) (Profile, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(profiles) == 1 {
			return profiles[0], true
		}
		return Profile{}, false
	}
	for _, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Name), name) || p.ID == name {
			return p, true
		}
	}
	for _, p := range profiles {
		if strings.EqualFold(friendlyName(p.Name), name) {
			return p, true
		}
	}
	var partial []Profile
	lower := strings.ToLower(name)
	for _, p := range profiles {
		if strings.Contains(strings.ToLower(p.Name), lower) {
			partial = append(partial, p)
		}
	}
	if len(partial) == 1 {
		return partial[0], true
	}
	return Profile{}, false
}

// friendlyName returns the text inside the trailing "[...]" of a listed
// profile name, or the name itself when there is none.
func friendlyName(listed string) string {
	listed = strings.TrimSpace(listed)
	open := strings.LastIndex(listed, "[")
	if open < 0 || !strings.HasSuffix(listed, "]") {
		return listed
	}
	return strings.TrimSpace(listed[open+1 : len(listed)-1])
}

// peersSharingSession counts imported profiles that would match the same live
// session (same host and username).
func peersSharingSession(profiles []Profile, sess Session) int {
	n := 0
	for _, p := range profiles {
		if sess.matchesTarget(p.Host, p.Username) {
			n++
		}
	}
	return n
}

func statusFromLog(ctx context.Context, cliPath, logPath string, profile domain.VPNProfile) (*domain.VPNStatus, bool) {
	logText, err := readLogTail(logPath)
	if err != nil {
		return nil, false
	}
	sess, active := replayLog(logText)
	if !active {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: "OpenVPN Connect has no active tunnel.",
		}, true
	}
	iface, up := live(sess.AssignedIP)
	if !up {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: "OpenVPN Connect log shows a tunnel, but the address is not on a local interface.",
		}, true
	}
	sess.Interface = iface

	imported, err := listProfiles(ctx, cliPath)
	if err != nil {
		imported = nil
	}
	target, ok := profileByName(imported, profile.VendorProfile)
	if !ok && strings.TrimSpace(profile.VendorProfile) != "" {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: fmt.Sprintf("Profile “%s” was not found in OpenVPN Connect.", profile.VendorProfile),
		}, true
	}
	if ok && !sess.matchesTarget(target.Host, target.Username) {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: fmt.Sprintf("OpenVPN Connect is up as %s@%s, not “%s”.", sess.Username, sess.Host, profile.VendorProfile),
		}, true
	}
	if ok && peersSharingSession(imported, sess) > 1 {
		if lastOpenedProfileID() != profile.ID {
			return &domain.VPNStatus{
				State:  domain.StateDisconnected,
				Known:  true,
				Detail: "OpenVPN Connect is up for another card that shares this host and account.",
			}, true
		}
	}
	return &domain.VPNStatus{
		State:      domain.StateConnected,
		AssignedIP: sess.AssignedIP,
		Interface:  sess.Interface,
		Identity:   sess.Username,
		Known:      true,
		Detail:     sessionDetail(sess),
	}, true
}

func vendorOptions(ctx context.Context, cliPath, logPath string) ([]domain.VendorProfileOption, error) {
	imported, err := listProfiles(ctx, cliPath)
	if err != nil {
		return nil, err
	}
	var active Session
	activeOK := false
	if logText, err := readLogTail(logPath); err == nil {
		active, activeOK = replayLog(logText)
	}
	out := make([]domain.VendorProfileOption, 0, len(imported))
	for _, p := range imported {
		detail := p.Host
		if p.Username != "" {
			if detail != "" {
				detail += " · "
			}
			detail += p.Username
		}
		if p.Type != "" {
			if detail != "" {
				detail += " · "
			}
			detail += p.Type
		}
		connected := false
		if activeOK {
			connected = active.matchesTarget(p.Host, p.Username)
			if connected {
				if _, up := live(active.AssignedIP); !up {
					connected = false
				}
			}
		}
		out = append(out, domain.VendorProfileOption{
			Name:      p.Name,
			Detail:    detail,
			Connected: connected,
		})
	}
	return out, nil
}