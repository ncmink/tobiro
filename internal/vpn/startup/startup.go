// Package startup toggles whether a vendor VPN GUI launches at login.
//
// It targets login items and user LaunchAgents only. Privileged tunnel helpers
// stay running so Connect still works when the user asks for it.
package startup

import "tobiro/internal/domain"

// Spec describes how one vendor client registers itself at login.
type Spec struct {
	// Labels are launchctl GUI-domain service labels (SMAppService / LaunchAgent).
	Labels []string

	// PrefDomain/PrefKey are an optional defaults boolean the vendor GUI itself
	// reads (e.g. TailscaleStartOnLogin). Empty means unused.
	PrefDomain string
	PrefKey    string

	// LoginItemNames are classic System Events login-item names.
	LoginItemNames []string
	// AppPath is the .app bundle used when adding a classic login item.
	AppPath string
}

// Unsupported is returned on platforms that have no login-item control.
func Unsupported() domain.StartupState {
	return domain.StartupState{
		Known:  false,
		Detail: "Start at login can be changed on macOS.",
	}
}
