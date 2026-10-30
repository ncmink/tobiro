//go:build darwin

package forcepoint

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nativefill"
)

const (
	fillAttemptTimeout = 12 * time.Second
	osascriptPath      = "/usr/bin/osascript"
)

// FillNativeLogin types the stored username and password into Forcepoint's
// login window. Forcepoint's fields are direct children of the window or a
// group, not a focused accessibility element, so this does not depend on
// which field the user clicked before pressing ⌘I.
func (p *Provider) FillNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return nil
	}
	if err := nativefill.ValidateFillValue("username", credential.Username); err != nil {
		return err
	}
	if err := nativefill.ValidateFillValue("password", credential.Secret); err != nil {
		return err
	}

	result, stderr, err := runFillScript(ctx, credential.Username, credential.Secret)
	if err != nil {
		if nativefill.IsAssistiveAccessError(stderr + err.Error()) {
			return domain.ErrAssistiveAccess
		}
		return fmt.Errorf("%w: %s", errFillWaiting, strings.TrimSpace(stderr+" "+err.Error()))
	}
	switch strings.TrimSpace(result) {
	case "filled":
		return nil
	default:
		if nativefill.IsAssistiveAccessError(stderr) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
}

// ProbeNativeLogin reports whether Forcepoint's login window exposes a username
// field. A nil error means the user can press the fill shortcut.
func (p *Provider) ProbeNativeLogin(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	result, stderr, err := runProbeScript(ctx)
	if err != nil {
		if nativefill.IsAssistiveAccessError(stderr + err.Error()) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
	if strings.TrimSpace(result) == "ready" {
		return nil
	}
	return errFillWaiting
}

func runProbeScript(ctx context.Context) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildProbeScript())
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

func buildProbeScript() string {
	return `
on hasUsernameField(theUI)
	tell application "System Events"
		try
			set textFields to UI elements of theUI whose role is "AXTextField"
			if (count of textFields) >= 1 then return true
		end try
	end tell
	return false
end hasUsernameField

on scanUI(theUI)
	if my hasUsernameField(theUI) then return true
	tell application "System Events"
		try
			repeat with g in groups of theUI
				if my hasUsernameField(g) then return true
			end repeat
		end try
		try
			repeat with sh in sheets of theUI
				if my hasUsernameField(sh) then return true
			end repeat
		end try
	end tell
	return false
end scanUI

on run
	tell application "System Events"
		repeat with procRef in (every process whose background only is false)
			set pname to (name of procRef) as string
			if pname contains "Forcepoint" or pname contains "Stonesoft" or pname contains "Sgvpn" or pname contains "sgvpn" then
				tell procRef
					repeat with w in windows
						if my scanUI(w) then return "ready"
					end repeat
				end tell
			end if
		end repeat
	end tell
	return "waiting"
end run
`
}

func runFillScript(ctx context.Context, username, password string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, fillAttemptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(username, password))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

// buildFillScript is the scan that previously filled Forcepoint: username via
// AX value, password via keystroke because the secure field ignores set value.
func buildFillScript(username, password string) string {
	return fmt.Sprintf(`
on tryFill(theUI, theUser, thePass)
	tell application "System Events"
		try
			set textFields to UI elements of theUI whose role is "AXTextField"
			set secureFields to UI elements of theUI whose role is "AXSecureTextField"
			set nText to count of textFields
			set nSecure to count of secureFields
			if nText >= 1 and nSecure >= 1 then
				if theUser is not "" then set value of item 1 of textFields to theUser
				if thePass is not "" then
					set focused of item 1 of secureFields to true
					delay 0.05
					keystroke "a" using command down
					delay 0.05
					keystroke thePass
				end if
				return true
			end if
			if nText >= 2 then
				if theUser is not "" then set value of item 1 of textFields to theUser
				if thePass is not "" then set value of item 2 of textFields to thePass
				return true
			end if
			if nText >= 1 and theUser is not "" and thePass is "" then
				set value of item 1 of textFields to theUser
				return true
			end if
			if nSecure >= 1 and thePass is not "" and theUser is "" then
				set focused of item 1 of secureFields to true
				delay 0.05
				keystroke "a" using command down
				delay 0.05
				keystroke thePass
				return true
			end if
		end try
	end tell
	return false
end tryFill

on run
	set theUser to %s
	set thePass to %s
	tell application "System Events"
		repeat with procRef in (every process whose background only is false)
			set pname to (name of procRef) as string
			if pname contains "Forcepoint" or pname contains "Stonesoft" or pname contains "Sgvpn" or pname contains "sgvpn" then
				try
					set frontmost of procRef to true
				end try
				tell procRef
					repeat with w in windows
						if my tryFill(w, theUser, thePass) then return "filled"
						repeat with g in groups of w
							if my tryFill(g, theUser, thePass) then return "filled"
						end repeat
						repeat with sh in sheets of w
							if my tryFill(sh, theUser, thePass) then return "filled"
						end repeat
					end repeat
				end tell
			end if
		end repeat
	end tell
	return "waiting"
end run
`, nativefill.QuoteAppleScript(username), nativefill.QuoteAppleScript(password))
}
