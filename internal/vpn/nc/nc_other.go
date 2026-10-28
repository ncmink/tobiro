//go:build !darwin

package nc

import (
	"context"
	"errors"
)

// ErrUnsupported is returned off macOS, where there is no scutil to ask.
var ErrUnsupported = errors.New("macOS network connections are only available on macOS")

// Supported reports that this platform has no macOS network connections.
func Supported() bool { return false }

func List(context.Context) ([]Service, error) { return nil, ErrUnsupported }

func Status(context.Context, string) (Reading, error) { return Reading{}, ErrUnsupported }

func Start(context.Context, string) error { return ErrUnsupported }

func Stop(context.Context, string) error { return ErrUnsupported }
