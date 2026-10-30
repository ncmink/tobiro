//go:build darwin

package forticlient

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
	fillWaitSeconds = 8
	scriptSlack     = 15 * time.Second
	osascriptPath   = "/usr/bin/osascript"
)

// FillNativeLogin types the stored username and password into FortiClient.
// The login form is an Electron web view, so the script wakes its accessibility
// tree and finds the fields itself. It never clicks Connect. Paste is not used:
// FortiClient rejects it.
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

	ctx, cancel := context.WithTimeout(ctx, time.Duration(fillWaitSeconds)*time.Second+scriptSlack)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(credential.Username, credential.Secret, fillWaitSeconds))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stderr := strings.TrimSpace(errBuf.String())
	if nativefill.IsAssistiveAccessError(stderr) || (err != nil && nativefill.IsAssistiveAccessError(err.Error())) {
		return domain.ErrAssistiveAccess
	}
	if err != nil {
		if stderr == "" {
			stderr = err.Error()
		}
		return fmt.Errorf("%w: %s", errFillWaiting, stderr)
	}
	if strings.TrimSpace(out.String()) == "filled" {
		return nil
	}
	return errFillWaiting
}

// ProbeNativeLogin reports whether FortiClient's login form exposes a username
// field. A nil error means the user can press the fill shortcut.
func (p *Provider) ProbeNativeLogin(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildProbeScript())
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stderr := strings.TrimSpace(errBuf.String())
	if nativefill.IsAssistiveAccessError(stderr) || (err != nil && nativefill.IsAssistiveAccessError(err.Error())) {
		return domain.ErrAssistiveAccess
	}
	if err != nil {
		return errFillWaiting
	}
	if strings.TrimSpace(out.String()) == "ready" {
		return nil
	}
	return errFillWaiting
}

func buildProbeScript() string {
	return `
global didWake

on labelOf(el)
	tell application "System Events"
		set bits to ""
		try
			set bits to bits & (description of el as string) & " "
		end try
		try
			set bits to bits & (title of el as string) & " "
		end try
		try
			set bits to bits & (value of attribute "AXPlaceholderValue" of el as string) & " "
		end try
		try
			set bits to bits & (help of el as string)
		end try
		return bits
	end tell
end labelOf

on isSecure(el)
	tell application "System Events"
		try
			if (role of el as string) is "AXSecureTextField" then return true
		end try
		try
			if (subrole of el as string) is "AXSecureTextField" then return true
		end try
	end tell
	return false
end isSecure

on isUsable(el)
	tell application "System Events"
		try
			if enabled of el is false then return false
		end try
		try
			set s to size of el
			if (item 1 of s) < 2 or (item 2 of s) < 2 then return false
		end try
	end tell
	return true
end isUsable

on skipField(el)
	set bits to my labelOf(el)
	if bits contains "token" or bits contains "Token" then return true
	if bits contains "confirm" or bits contains "Confirm" then return true
	if bits contains "new password" or bits contains "New Password" or bits contains "New password" then return true
	return false
end skipField

on tryDetectUsername(theUI)
	set wtitle to ""
	tell application "System Events"
		try
			set wtitle to name of theUI as string
		end try
	end tell
	if wtitle contains "Input password" then return false
	if wtitle contains "New Password" then return false
	if wtitle contains "Token" then return false

	set everything to {}
	try
		tell application "System Events"
			set everything to entire contents of theUI
		end tell
	on error
		return false
	end try

	set textFields to {}
	repeat with el in everything
		if my isUsable(el) and not my skipField(el) and not my isSecure(el) then
			set isText to false
			tell application "System Events"
				try
					set isText to ((role of el as string) is "AXTextField")
				end try
			end tell
			if isText then
				set bits to my labelOf(el)
				if bits contains "pass" or bits contains "Pass" then
					-- password text field, not username
				else
					set end of textFields to el
				end if
			end if
		end if
	end repeat

	if (count of textFields) >= 1 then
		repeat with el in textFields
			set bits to my labelOf(el)
			if bits contains "user" or bits contains "User" then return true
		end repeat
		return true
	end if
	return false
end tryDetectUsername

on scanProcess(pname)
	tell application "System Events"
		set wins to windows of process pname
	end tell
	repeat with w in wins
		if my tryDetectUsername(w) then return true
		set shs to {}
		tell application "System Events"
			try
				set shs to sheets of w
			end try
		end tell
		repeat with sh in shs
			if my tryDetectUsername(sh) then return true
		end repeat
	end repeat
	return false
end scanProcess

on wakeElectron(pname)
	tell application "System Events"
		try
			set value of attribute "AXManualAccessibility" of process pname to true
		end try
		try
			set value of attribute "AXEnhancedUserInterface" of process pname to true
		end try
	end tell
	delay 0.8
end wakeElectron

on webViewHidden(pname)
	tell application "System Events"
		try
			if (value of attribute "AXEnhancedUserInterface" of process pname) is true then return false
		end try
	end tell
	return true
end webViewHidden

on scanOnce()
	repeat with wanted in {"FortiClientAgent", "FortiClient"}
		set pname to wanted as string
		set isUp to false
		tell application "System Events"
			set isUp to (exists process pname)
		end tell
		if isUp then
			if pname is "FortiClient" and didWake is false and my webViewHidden(pname) then
				set didWake to true
				my wakeElectron(pname)
			end if
			if my scanProcess(pname) then return true
		end if
	end repeat
	return false
end scanOnce

on run
	set didWake to false
	if my scanOnce() then return "ready"
	return "waiting"
end run
`
}

func buildFillScript(username, password string, waitSeconds int) string {
	return fmt.Sprintf(`
global didWake
on labelOf(el)
	tell application "System Events"
		set bits to ""
		try
			set bits to bits & (description of el as string) & " "
		end try
		try
			set bits to bits & (title of el as string) & " "
		end try
		try
			set bits to bits & (value of attribute "AXPlaceholderValue" of el as string) & " "
		end try
		try
			set bits to bits & (help of el as string)
		end try
		return bits
	end tell
end labelOf

on isSecure(el)
	tell application "System Events"
		try
			if (role of el as string) is "AXSecureTextField" then return true
		end try
		try
			if (subrole of el as string) is "AXSecureTextField" then return true
		end try
	end tell
	return false
end isSecure

on isUsable(el)
	tell application "System Events"
		try
			if enabled of el is false then return false
		end try
		try
			set s to size of el
			if (item 1 of s) < 2 or (item 2 of s) < 2 then return false
		end try
	end tell
	return true
end isUsable

on skipField(el)
	set bits to my labelOf(el)
	if bits contains "token" or bits contains "Token" then return true
	if bits contains "confirm" or bits contains "Confirm" then return true
	if bits contains "new password" or bits contains "New Password" or bits contains "New password" then return true
	return false
end skipField

on typeSecret(el, theText)
	tell application "System Events"
		try
			set value of el to theText
		end try
		try
			perform action "AXPress" of el
		end try
		set focused of el to true
		delay 0.05
		keystroke "a" using command down
		delay 0.05
		keystroke theText
	end tell
end typeSecret

on tryFill(theUI, theUser, thePass, pname)
	set wtitle to ""
	tell application "System Events"
		try
			set wtitle to name of theUI as string
		end try
	end tell
	if wtitle contains "Input password" then return false
	if wtitle contains "New Password" then return false
	if wtitle contains "Token" then return false

	set everything to {}
	try
		tell application "System Events"
			set everything to entire contents of theUI
		end tell
	on error
		return false
	end try

	set textFields to {}
	set secureFields to {}
	set passwordTexts to {}
	repeat with el in everything
		if my isUsable(el) and not my skipField(el) then
			if my isSecure(el) then
				set end of secureFields to el
			else
				set isText to false
				tell application "System Events"
					try
						set isText to ((role of el as string) is "AXTextField")
					end try
				end tell
				if isText then
					set bits to my labelOf(el)
					if bits contains "pass" or bits contains "Pass" then
						set end of passwordTexts to el
					else
						set end of textFields to el
					end if
				end if
			end if
		end if
	end repeat

	set passField to missing value
	set passY to 1000000
	if (count of secureFields) > 0 then
		repeat with el in secureFields
			set ey to 0
			tell application "System Events"
				try
					set ey to item 2 of (position of el)
				end try
			end tell
			if passField is missing value or ey < passY then
				set passY to ey
				set passField to el
			end if
		end repeat
	else if (count of passwordTexts) > 0 then
		repeat with el in passwordTexts
			set ey to 0
			tell application "System Events"
				try
					set ey to item 2 of (position of el)
				end try
			end tell
			if passField is missing value or ey < passY then
				set passY to ey
				set passField to el
			end if
		end repeat
	else if thePass is not "" and (count of textFields) is 2 then
		set lower to missing value
		set lowerY to -1
		repeat with el in textFields
			set ey to 0
			tell application "System Events"
				try
					set ey to item 2 of (position of el)
				end try
			end tell
			if ey > lowerY then
				set lowerY to ey
				set lower to el
			end if
		end repeat
		set passField to lower
		set passY to lowerY
	end if
	if thePass is not "" and passField is missing value then return false

	set userField to missing value
	if theUser is not "" then
		if passField is not missing value then
			set bestDist to 1000000
			repeat with el in textFields
				set ey to passY
				tell application "System Events"
					try
						set ey to item 2 of (position of el)
					end try
				end tell
				if ey < passY then
					set dist to passY - ey
					if dist < bestDist then
						set bestDist to dist
						set userField to el
					end if
				end if
			end repeat
		end if
		if userField is missing value then
			repeat with el in textFields
				set bits to my labelOf(el)
				if bits contains "user" or bits contains "User" then
					set userField to el
					exit repeat
				end if
			end repeat
		end if
		if userField is missing value and (count of textFields) is 1 then
			set userField to item 1 of textFields
		end if
		if userField is missing value then return false
	end if

	if not my bringFront(pname) then return false
	delay 0.1
	if userField is not missing value and theUser is not "" then
		my typeSecret(userField, theUser)
	end if
	if passField is not missing value and thePass is not "" then
		my typeSecret(passField, thePass)
	end if
	return true
end tryFill

on bringFront(pname)
	tell application "System Events"
		try
			set frontmost of process pname to true
		end try
		repeat 30 times
			try
				if frontmost of process pname then return true
			end try
			delay 0.05
		end repeat
	end tell
	return false
end bringFront

on wakeElectron(pname)
	tell application "System Events"
		try
			set value of attribute "AXManualAccessibility" of process pname to true
		end try
		try
			set value of attribute "AXEnhancedUserInterface" of process pname to true
		end try
	end tell
	delay 0.8
end wakeElectron

on webViewHidden(pname)
	tell application "System Events"
		try
			if (value of attribute "AXEnhancedUserInterface" of process pname) is true then return false
		end try
	end tell
	return true
end webViewHidden

on scanProcess(pname, theUser, thePass)
	tell application "System Events"
		set wins to windows of process pname
	end tell
	repeat with w in wins
		if my tryFill(w, theUser, thePass, pname) then return true
		set shs to {}
		tell application "System Events"
			try
				set shs to sheets of w
			end try
		end tell
		repeat with sh in shs
			if my tryFill(sh, theUser, thePass, pname) then return true
		end repeat
	end repeat
	return false
end scanProcess

on scanOnce(theUser, thePass)
	repeat with wanted in {"FortiClientAgent", "FortiClient"}
		set pname to wanted as string
		set isUp to false
		tell application "System Events"
			set isUp to (exists process pname)
		end tell
		if isUp then
			if pname is "FortiClient" and didWake is false and my webViewHidden(pname) then
				set didWake to true
				my wakeElectron(pname)
			end if
			if my scanProcess(pname, theUser, thePass) then return true
		end if
	end repeat
	return false
end scanOnce

on run
	set didWake to false
	set waitFor to %d
	set theUser to %s
	set thePass to %s
	set deadline to (current date) + waitFor
	repeat
		if my scanOnce(theUser, thePass) then return "filled"
		if (current date) > deadline then exit repeat
		delay 0.25
	end repeat
	return "waiting"
end run
`, waitSeconds, nativefill.QuoteAppleScript(username), nativefill.QuoteAppleScript(password))
}
