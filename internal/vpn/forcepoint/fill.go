package forcepoint

import (
	"errors"
	"strconv"
	"strings"

	"tobiro/internal/domain"
)

// errFillWaiting means the Forcepoint process is up but the login fields are
// not on screen yet (the user may still be picking a gateway).
var errFillWaiting = errors.New("forcepoint login window is not visible yet")

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
