//go:build !darwin

package doctor

import "context"

// ReadInetTable is not implemented off macOS yet.
func ReadInetTable(_ context.Context) ([]InetRoute, error) {
	return nil, nil
}
