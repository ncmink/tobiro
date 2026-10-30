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
)

const (
	fillAttemptTimeout = 12 * time.Second
	osascriptPath      = "/usr/bin/osascript"
)

// FillNativeLogin types the stored username and password into Forcepoint's
// login window. It never clicks Connect.
func (p *Provider) FillNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return nil
	}
	if err := validateFillValue("username", credential.Username); err != nil {
		return err
	}
	if err := validateFillValue("password", credential.Secret); err != nil {
		return err
	}

	result, stderr, err := runFillScript(ctx, credential.Username, credential.Secret)
	if err != nil {
		if isAssistiveAccessError(stderr + err.Error()) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
	switch strings.TrimSpace(result) {
	case "filled":
		return nil
	case "waiting":
		return errFillWaiting
	default:
		if isAssistiveAccessError(stderr) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
}

// ProbeNativeLogin reports whether Forcepoint's login fields are on screen.
func (p *Provider) ProbeNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return errFillWaiting
	}
	if err := validateFillValue("username", credential.Username); err != nil {
		return err
	}
	if err := validateFillValue("password", credential.Secret); err != nil {
		return err
	}

	result, stderr, err := runProbeScript(ctx, credential.Username, credential.Secret)
	if err != nil {
		if isAssistiveAccessError(stderr + err.Error()) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
	switch strings.TrimSpace(result) {
	case "ready":
		return nil
	case "waiting":
		return errFillWaiting
	default:
		if isAssistiveAccessError(stderr) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
}

func runProbeScript(ctx context.Context, username, password string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, fillAttemptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(username, password, true))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

func runFillScript(ctx context.Context, username, password string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, fillAttemptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(username, password, false))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}

func buildFillScript(username, password string, probeOnly bool) string {
	return fmt.Sprintf(`
on tryFill(theUI, theUser, thePass, probeOnly)
	tell application "System Events"
		try
			set textFields to UI elements of theUI whose role is "AXTextField"
			set secureFields to UI elements of theUI whose role is "AXSecureTextField"
			set nText to count of textFields
			set nSecure to count of secureFields
			if nText >= 1 and nSecure >= 1 then
				if probeOnly then return true
				if theUser is not "" then set value of item 1 of textFields to theUser
				if thePass is not "" then
					try
						set value of item 1 of secureFields to thePass
					end try
					set focused of item 1 of secureFields to true
					delay 0.05
					keystroke "a" using command down
					delay 0.05
					keystroke thePass
				end if
				return true
			end if
			if nText >= 2 then
				if probeOnly then return true
				if theUser is not "" then set value of item 1 of textFields to theUser
				if thePass is not "" then set value of item 2 of textFields to thePass
				return true
			end if
			if nText >= 1 and theUser is not "" and thePass is "" then
				if probeOnly then return true
				set value of item 1 of textFields to theUser
				return true
			end if
			if nSecure >= 1 and thePass is not "" and theUser is "" then
				if probeOnly then return true
				try
					set value of item 1 of secureFields to thePass
				end try
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
	set probeOnly to %t
	set theUser to %s
	set thePass to %s
	tell application "System Events"
		repeat with procRef in (every process whose background only is false)
			set pname to (name of procRef) as string
			if pname contains "Forcepoint" or pname contains "Stonesoft" or pname contains "Sgvpn" or pname contains "sgvpn" then
				tell procRef
					repeat with w in windows
						if my tryFill(w, theUser, thePass, probeOnly) then
							if probeOnly then return "ready"
							return "filled"
						end if
						repeat with g in groups of w
							if my tryFill(g, theUser, thePass, probeOnly) then
								if probeOnly then return "ready"
								return "filled"
							end if
						end repeat
						repeat with sh in sheets of w
							if my tryFill(sh, theUser, thePass, probeOnly) then
								if probeOnly then return "ready"
								return "filled"
							end if
						end repeat
					end repeat
				end tell
			end if
		end repeat
	end tell
	return "waiting"
end run
`, probeOnly, quoteAppleScript(username), quoteAppleScript(password))
}
