//go:build windows

package openvpn

import (
	"context"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/tunnel"
)

func statusSupported() bool { return true }

func readStatus(ctx context.Context, p *Provider, profile domain.VPNProfile) *domain.VPNStatus {
	p.detectPaths(ctx)
	if p.cliPath == "" {
		return &domain.VPNStatus{Known: false}
	}
	logPath := platformLogPath()
	if st, ok := statusFromLog(ctx, p.cliPath, logPath, profile); ok {
		return st
	}
	return adapterStatus(ctx, p, profile)
}

func adapterStatus(ctx context.Context, p *Provider, profile domain.VPNProfile) *domain.VPNStatus {
	imported, err := listProfiles(ctx, p.cliPath)
	if err != nil {
		return &domain.VPNStatus{Known: false}
	}
	target, ok := profileByName(imported, profile.VendorProfile)
	if !ok {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: "Could not read OpenVPN Connect status.",
		}
	}
	host := strings.TrimSpace(target.Host)
	if host == "" {
		return &domain.VPNStatus{Known: false}
	}
	reading := tunnel.ProbeWith(ctx, tunnel.Options{
		Host:           host,
		Port:           1194,
		ProcessHints:   []string{"OpenVPN", "ovpn"},
		AdapterHints:   []string{"OpenVPN", "TAP-Windows", "tap-windows"},
		RequireAdapter: true,
	})
	if !reading.Connected {
		return tunnel.Status(reading)
	}
	if peersSharingSession(imported, Session{Host: host, Username: target.Username, Resolved: host}) > 1 {
		if lastOpenedProfileID() != profile.ID {
			return &domain.VPNStatus{
				State:  domain.StateDisconnected,
				Known:  true,
				Detail: "An OpenVPN tunnel is up for another card on this host.",
			}
		}
	}
	return tunnel.Status(reading)
}