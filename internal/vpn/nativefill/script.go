package nativefill

import (
	"errors"
	"strconv"
	"strings"
)

// ErrNoFocus means the vendor login field was not focused when fill ran.
var ErrNoFocus = errors.New("click a username or password field in the vendor login window, then press ⌘I")

// ErrNoVendorApp means none of the expected vendor processes are running.
var ErrNoVendorApp = errors.New("vendor app is not running")

// ErrNotEditable means focus is on something other than a text field.
var ErrNotEditable = errors.New("focus is not in a username or password field")

func ValidateFillValue(label, value string) error {
	if strings.ContainsAny(value, "\n\r\x00") {
		return errors.New(label + " contains illegal control characters")
	}
	return nil
}

// QuoteAppleScript encodes s as an AppleScript string expression.
func QuoteAppleScript(s string) string {
	if s == "" {
		return `""`
	}
	ids := make([]string, 0, len(s))
	for _, r := range s {
		ids = append(ids, strconv.FormatInt(int64(r), 10))
	}
	return "character id {" + strings.Join(ids, ", ") + "}"
}

func IsAssistiveAccessError(stderr string) bool {
	msg := strings.ToLower(stderr)
	return strings.Contains(msg, "not allowed assistive access") ||
		strings.Contains(msg, "not authorized to send apple events") ||
		strings.Contains(msg, "osascript is not allowed") ||
		strings.Contains(msg, "(-1719)") ||
		strings.Contains(msg, "(-1743)")
}
