//go:build !darwin

package tunnel

import "context"

// Supported reports whether tunnel probing is implemented on this platform.
func Supported() bool { return false }

// ProbeWith is not implemented outside macOS yet.
func ProbeWith(_ context.Context, _ Options) Reading {
	return Reading{Detail: "Tunnel status is not read on this platform."}
}

func probeVendor(_ context.Context, _ []string) Reading {
	return Reading{Detail: "Tunnel status is not read on this platform."}
}

func invalidateSnapshot() {}
