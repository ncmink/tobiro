package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrClientNotInstalled   = errors.New("vendor VPN client is not installed")
	ErrSecretNotFound       = errors.New("no stored password for this credential")
	ErrInteractionCancelled = errors.New("the user cancelled the request")
	ErrAssistiveAccess      = errors.New("Tobiro needs Accessibility permission to fill the vendor login fields. Open System Settings → Privacy & Security → Accessibility and enable Tobiro")
)

// UnsupportedOperationError is returned when an adapter deliberately refuses an
// operation because the installed client/platform/version does not officially
// support it.
//
// This is the "fail safely instead of guessing" path: Tobiro never
// invents undocumented CLI arguments. The UI turns this into an explanation
// plus an "Open native client" action.
type UnsupportedOperationError struct {
	Provider  VPNProvider
	Operation string
	Platform  string
	Reason    string
	// NativeUIFallback indicates the user can complete the action in the
	// vendor GUI instead.
	NativeUIFallback bool
}

func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("%s: %s is not supported on %s: %s",
		e.Provider.DisplayName(), e.Operation, e.Platform, e.Reason)
}

// IsUnsupported reports whether err is an UnsupportedOperationError.
func IsUnsupported(err error) bool {
	var target *UnsupportedOperationError
	return errors.As(err, &target)
}

// AsUnsupported extracts the typed error when present.
func AsUnsupported(err error) (*UnsupportedOperationError, bool) {
	var target *UnsupportedOperationError
	ok := errors.As(err, &target)
	return target, ok
}
