package forticlient

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/vpn"
)

// errFillWaiting means FortiClient is up but the username/password fields are
// not on screen yet.
var errFillWaiting = errors.New("forticlient login window is not visible yet")

// Provider implements vpn.LoginFiller. The method bodies are platform-specific.
var _ vpn.LoginFiller = (*Provider)(nil)

func canFill(credential domain.Credential) bool {
	return strings.TrimSpace(credential.Username) != "" || credential.Secret != ""
}

func validateFillValue(label, value string) error {
	if strings.ContainsAny(value, "\n\r\x00") {
		return errors.New(label + " contains illegal control characters")
	}
	return nil
}

// quoteAppleScript encodes s as an AppleScript string expression. Character
// codes are used so quotes, backslashes and unicode cannot break the script.
// The script is sent to osascript on stdin, never as a process argument.
func quoteAppleScript(s string) string {
	if s == "" {
		return `""`
	}
	ids := make([]string, 0, len(s))
	for _, r := range s {
		ids = append(ids, strconv.FormatInt(int64(r), 10))
	}
	return "character id {" + strings.Join(ids, ", ") + "}"
}

func isAssistiveAccessError(stderr string) bool {
	msg := strings.ToLower(stderr)
	return strings.Contains(msg, "not allowed assistive access") ||
		strings.Contains(msg, "not authorized to send apple events") ||
		strings.Contains(msg, "osascript is not allowed") ||
		strings.Contains(msg, "(-1719)") ||
		strings.Contains(msg, "(-1743)")
}

// buildFillScript types into FortiClient's login fields.
//
// FortiClient's password control ignores the pasteboard (the native
// NSSecureTextField and the Electron password input both do). The script
// keystrokes the secret instead, after selecting whatever is already there.
// It never presses Return and never clicks Connect: those submit the dialog.
//
// A token prompt and the "new password" dialog are left alone. A lone secure
// field is filled only when it is labelled as a password, so a FortiToken box
// does not receive the VPN password.
func buildFillScript(username, password string) string {
	return fmt.Sprintf(`
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

on accessDenied(errMsg)
	set msg to errMsg as string
	if msg contains "assistive" or msg contains "not authorized" or msg contains "(-1719)" or msg contains "(-1743)" then return true
	return false
end accessDenied

on typeSecret(el, thePass)
	tell application "System Events"
		try
			set value of el to thePass
		end try
		try
			perform action "AXPress" of el
		end try
		set focused of el to true
		delay 0.05
		keystroke "a" using command down
		delay 0.05
		keystroke thePass
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
	on error errMsg number errNum
		if my accessDenied(errMsg) then error errMsg number errNum
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
		set upper to missing value
		set upperY to 1000000
		set lower to missing value
		set lowerY to -1
		repeat with el in textFields
			set ey to 0
			tell application "System Events"
				try
					set ey to item 2 of (position of el)
				end try
			end tell
			if ey < upperY then
				set upperY to ey
				set upper to el
			end if
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

	if passField is missing value then
		if thePass is not "" then return false
	else if userField is missing value then
		set ok to false
		if wtitle contains "Password" then set ok to true
		set bits to my labelOf(passField)
		if bits contains "pass" or bits contains "Pass" then set ok to true
		if ok is false then return false
	end if

	tell application "System Events"
		tell process pname
			set frontmost to true
		end tell
	end tell
	delay 0.12
	if userField is not missing value and theUser is not "" then
		my typeSecret(userField, theUser)
	end if
	if passField is not missing value and thePass is not "" then
		my typeSecret(passField, thePass)
	end if
	return true
end tryFill

on run
	set theUser to %s
	set thePass to %s
	set namesToTry to {"FortiClientAgent", "FortiClient"}
	tell application "System Events"
		repeat with wanted in namesToTry
			set wantedName to wanted as string
			repeat with procRef in (every process whose background only is false)
				set pname to (name of procRef) as string
				if pname is wantedName then
					-- FortiClient is Electron. Its password field stays out of the
					-- accessibility tree until this flag is toggled, and the tree
					-- is only readable after a short pause.
					if pname is "FortiClient" then
						tell procRef
							try
								set value of attribute "AXEnhancedUserInterface" to false
							end try
							delay 0.25
							try
								set value of attribute "AXEnhancedUserInterface" to true
							end try
						end tell
						delay 1
					end if
					tell procRef
						repeat with w in windows
							if my tryFill(w, theUser, thePass, pname) then return "filled"
							try
								repeat with sh in sheets of w
									if my tryFill(sh, theUser, thePass, pname) then return "filled"
								end repeat
							end try
						end repeat
					end tell
				end if
			end repeat
		end repeat
	end tell
	return "waiting"
end run
`, quoteAppleScript(username), quoteAppleScript(password))
}
