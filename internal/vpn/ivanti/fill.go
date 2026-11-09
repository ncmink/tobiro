package ivanti

import (
	"errors"
	"strings"

	"tobiro/internal/domain"
)

var errFillWaiting = errors.New("could not find Ivanti's username and password fields — open the login window, then press the fill shortcut")

var ivantiProcessHints = []string{
	"Pulse",
	"Ivanti",
	"Jam",
	"dsAccess",
}

func canFill(credential domain.Credential) bool {
	return strings.TrimSpace(credential.Username) != "" || credential.Secret != ""
}

// OwnsNativeProcess reports whether a process name belongs to Ivanti Secure Access.
func (p *Provider) OwnsNativeProcess(name string) bool {
	lower := strings.ToLower(name)
	for _, h := range ivantiProcessHints {
		if strings.Contains(lower, strings.ToLower(h)) {
			return true
		}
	}
	return false
}
