//go:build !darwin

package nativefill

import "context"

// FrontmostProcess is only supported on macOS.
func FrontmostProcess(_ context.Context) string { return "" }

// FillAtFocusedField is only supported on macOS.
func FillAtFocusedField(_ context.Context, _, _ []string, _, _ string) error {
	return ErrNoFocus
}
