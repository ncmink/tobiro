//go:build darwin

package nativefill

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
	osascriptPath = "/usr/bin/osascript"
	scriptSlack   = 12 * time.Second
)

// FrontmostProcess names the app that has keyboard focus, or "" if unknown.
func FrontmostProcess(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, osascriptPath, "-e",
		`tell application "System Events" to get name of first process whose frontmost is true`)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FillAtFocusedField types into the vendor field that already has keyboard
// focus when the user presses ⌘I (vendor app must be frontmost).
func FillAtFocusedField(ctx context.Context, exactProcessNames, processNameContains []string, username, password string) error {
	if err := ValidateFillValue("username", username); err != nil {
		return err
	}
	if err := ValidateFillValue("password", password); err != nil {
		return err
	}
	if len(exactProcessNames) == 0 && len(processNameContains) == 0 {
		return ErrNoVendorApp
	}

	wait := 15
	ctx, cancel := context.WithTimeout(ctx, time.Duration(wait)*time.Second+scriptSlack)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFocusFillScript(exactProcessNames, processNameContains, username, password))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stderr := strings.TrimSpace(errBuf.String())
	if IsAssistiveAccessError(stderr) || (err != nil && IsAssistiveAccessError(err.Error())) {
		return domain.ErrAssistiveAccess
	}
	if err != nil {
		if stderr == "" {
			stderr = err.Error()
		}
		return fmt.Errorf("%w: %s", ErrNoFocus, stderr)
	}

	switch strings.TrimSpace(out.String()) {
	case "filled":
		return nil
	case "no_app":
		return ErrNoVendorApp
	case "not_field":
		return ErrNotEditable
	case "no_pass_field":
		return fmt.Errorf("%w: could not find the password field in this login window", ErrNoFocus)
	case "no_focus", "waiting":
		return ErrNoFocus
	default:
		return ErrNoFocus
	}
}

func buildFocusFillScript(exactProcessNames, processNameContains []string, username, password string) string {
	names := make([]string, len(exactProcessNames))
	for i, n := range exactProcessNames {
		names[i] = QuoteAppleScript(n)
	}
	procList := strings.Join(names, ", ")
	hints := make([]string, len(processNameContains))
	for i, h := range processNameContains {
		hints[i] = QuoteAppleScript(h)
	}
	hintList := strings.Join(hints, ", ")

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
		try
		 set d to description of el as string
		 if d contains "secure" then return true
		end try
	end tell
	return false
end isSecure

on fieldEditable(el)
	tell application "System Events"
		try
		 set r to role of el as string
		 if r is "AXTextField" or r is "AXSecureTextField" or r is "AXTextArea" then return true
		end try
		try
		 set d to description of el as string
		 if d contains "text field" or d contains "secure" then return true
		end try
	end tell
	return false
end fieldEditable

on skipField(el)
	set bits to my labelOf(el)
	if bits contains "token" or bits contains "Token" then return true
	if bits contains "confirm" or bits contains "Confirm" then return true
	if bits contains "new password" or bits contains "New Password" then return true
	return false
end skipField

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

on focusedField(pname)
	tell application "System Events"
		try
		 set proc to process pname
		 return value of attribute "AXFocusedUIElement" of proc
		on error
		 return missing value
		end try
	end tell
end focusedField

on windowContaining(el)
	repeat 16 times
		if el is missing value then return missing value
		tell application "System Events"
			try
				set r to role of el as string
				if r is "AXWindow" or r is "AXSheet" then return el
				set el to value of attribute "AXParent" of el
			on error
				return missing value
			end try
		end tell
	end repeat
	return missing value
end windowContaining

on bringFront(pname)
	tell application "System Events"
		try
		 set frontmost of process pname to true
		end try
		repeat 40 times
			try
			 if frontmost of process pname then return true
			end try
			delay 0.05
		end repeat
	end tell
	return false
end bringFront

on typeSecret(el, theText)
	tell application "System Events"
		try
		 set focused of el to true
		end try
		delay 0.06
		try
		 set value of el to theText
		end try
		delay 0.05
		keystroke "a" using command down
		delay 0.05
		keystroke theText
	end tell
end typeSecret

on findLoginFields(container, thePass)
	set everything to {}
	try
		tell application "System Events"
			set everything to entire contents of container
		end tell
	on error
		return {missing value, missing value}
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
	end if
	if thePass is not "" and passField is missing value then return {missing value, missing value}

	set userField to missing value
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
	return {userField, passField}
end findLoginFields

on findRunning(processList, hintList)
	set found to my listRunning(processList, hintList)
	if (count of found) is 0 then return ""
	return item 1 of found
end findRunning

on listRunning(processList, hintList)
	set found to {}
	repeat with wanted in processList
		set pname to wanted as string
		if pname is not "" then
			tell application "System Events"
				if exists process pname then set end of found to pname
			end tell
		end if
	end repeat
	tell application "System Events"
		set procNames to name of every process whose background only is false
	end tell
	repeat with procName in procNames
		set n to procName as string
		repeat with hint in hintList
			set h to hint as string
			if h is not "" and n contains h then
				set end of found to n
				exit repeat
			end if
		end repeat
	end repeat
	return found
end listRunning

on fillLoginWindow(pname, focusedEl, theUser, thePass)
	if not my bringFront(pname) then return "waiting"
	delay 0.12
	set container to my windowContaining(focusedEl)
	if container is missing value then return "not_field"
	set fieldPair to my findLoginFields(container, thePass)
	set userField to item 1 of fieldPair
	set passField to item 2 of fieldPair
	if theUser is not "" and userField is missing value then return "not_field"
	if thePass is not "" and passField is missing value then return "no_pass_field"
	if userField is not missing value and theUser is not "" then
		my typeSecret(userField, theUser)
	end if
	if passField is not missing value and thePass is not "" then
		my typeSecret(passField, thePass)
	end if
	return "filled"
end fillLoginWindow

-- Electron (FortiClient) hides its web content from accessibility until asked.
on wakeAccessibility(pname)
	tell application "System Events"
		set woke to false
		try
			if (value of attribute "AXManualAccessibility" of process pname) is not true then
				set value of attribute "AXManualAccessibility" of process pname to true
				set woke to true
			end if
		end try
		try
			if (value of attribute "AXEnhancedUserInterface" of process pname) is not true then
				set value of attribute "AXEnhancedUserInterface" of process pname to true
				set woke to true
			end if
		end try
	end tell
	if woke then delay 0.6
end wakeAccessibility

on isFront(pname)
	tell application "System Events"
		try
			return frontmost of process pname
		end try
	end tell
	return false
end isFront

on frontWindow(pname)
	tell application "System Events"
		try
			return window 1 of process pname
		end try
	end tell
	return missing value
end frontWindow

on run
	set processList to {%s}
	set hintList to {%s}
	set theUser to %s
	set thePass to %s
	set candidates to my listRunning(processList, hintList)
	if (count of candidates) is 0 then return "no_app"
	repeat with pname in candidates
		set pname to pname as string
		my wakeAccessibility(pname)
		set el to my focusedField(pname)
		if el is not missing value and my fieldEditable(el) then
			return my fillLoginWindow(pname, el, theUser, thePass)
		end if
	end repeat
	-- Focus is in a web view the tree does not report as a text field: fill the
	-- login form of whichever vendor window is in front.
	repeat with pname in candidates
		set pname to pname as string
		if my isFront(pname) then
			set w to my frontWindow(pname)
			if w is not missing value then return my fillLoginWindow(pname, w, theUser, thePass)
		end if
	end repeat
	return "no_focus"
end run
`, procList, hintList, QuoteAppleScript(username), QuoteAppleScript(password))
}
