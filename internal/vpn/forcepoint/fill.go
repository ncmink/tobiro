package forcepoint

import (
	"errors"
	"strings"

	"tobiro/internal/domain"
)

// errFillWaiting means the Forcepoint login fields were not found as children
// of the window, a group, or a sheet.
var errFillWaiting = errors.New("could not find Forcepoint's username and password fields — open the login window, then press ⌘I")

var forcepointExactProcesses = []string{
	"Forcepoint NGFW VPN Client",
	"Forcepoint VPN Client",
}

var forcepointProcessHints = []string{
	"Forcepoint",
	"Stonesoft",
	"Sgvpn",
	"sgvpn",
}

func canFill(credential domain.Credential) bool {
	return strings.TrimSpace(credential.Username) != "" || credential.Secret != ""
}

// OwnsNativeProcess reports whether a macOS process name belongs to Forcepoint.
func (p *Provider) OwnsNativeProcess(name string) bool {
	for _, n := range forcepointExactProcesses {
		if name == n {
			return true
		}
	}
	for _, h := range forcepointProcessHints {
		if strings.Contains(name, h) {
			return true
		}
	}
	return false
}
