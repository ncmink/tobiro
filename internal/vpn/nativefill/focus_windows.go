//go:build windows

package nativefill

import (
	"context"

	"golang.org/x/sys/windows"
)

// FrontmostProcess returns the image name (without .exe) of the process that
// owns the foreground window, e.g. "sgvpn".
func FrontmostProcess(_ context.Context) string {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return ""
	}
	return windowProcess(hwnd)
}

// FillAtFocusedField is not used on Windows; vendor adapters fill through
// FindVendorWindows and LoginFields instead.
func FillAtFocusedField(_ context.Context, _, _ []string, _, _ string) error {
	return ErrNoFocus
}
