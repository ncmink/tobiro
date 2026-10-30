package forticlient

import (
	"errors"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/vpn"
)

// errFillWaiting means FortiClient is running but its login fields were not in
// the accessibility tree.
var errFillWaiting = errors.New("could not find FortiClient's username and password fields — open the login window, then press ⌘I")

var forticlientProcesses = []string{"FortiClientAgent", "FortiClient"}

// Provider implements vpn.LoginFiller. The method bodies are platform-specific.
var _ vpn.LoginFiller = (*Provider)(nil)

func canFill(credential domain.Credential) bool {
	return strings.TrimSpace(credential.Username) != "" || credential.Secret != ""
}

// OwnsNativeProcess reports whether a macOS process name belongs to FortiClient.
func (p *Provider) OwnsNativeProcess(name string) bool {
	return strings.Contains(name, "FortiClient")
}
