//go:build !darwin

package startup

import (
	"context"

	"tobiro/internal/domain"
)

// Read reports that start-at-login cannot be changed on this platform.
func Read(_ context.Context, _ Spec) (domain.StartupState, error) {
	return Unsupported(), nil
}

// Set is a no-op off macOS.
func Set(_ context.Context, _ Spec, _ bool) error {
	return nil
}
