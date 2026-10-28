//go:build darwin

package nc

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"tobiro/internal/sysexec"
)

const scutilPath = "/usr/sbin/scutil"

// serviceID guards the identifier before it becomes a command argument. Every
// value comes from scutil's own output, so this only has to reject anything that
// did not.
var serviceID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Supported reports whether this platform exposes macOS network connections.
func Supported() bool { return sysexec.Exists(scutilPath) }

// List returns every VPN service macOS knows about, both its own and those
// registered by third-party apps.
func List(ctx context.Context) ([]Service, error) {
	res, err := sysexec.Run(ctx, scutilPath, "--nc", "list")
	if err != nil && res.Stdout == "" {
		return nil, fmt.Errorf("list macOS VPN services: %s", failure(res, err))
	}
	return ParseList(res.Stdout), nil
}

// Status reads one service's live state.
func Status(ctx context.Context, id string) (Reading, error) {
	if err := validate(id); err != nil {
		return Reading{}, err
	}
	res, err := sysexec.Run(ctx, scutilPath, "--nc", "status", id)
	if err != nil && res.Stdout == "" {
		return Reading{}, fmt.Errorf("read macOS VPN status: %s", failure(res, err))
	}
	return ParseStatus(res.Stdout), nil
}

// Start asks macOS to bring a service up. It returns as soon as macOS accepts
// the request — the tunnel comes up asynchronously, so callers poll Status.
//
// No credentials are passed. scutil accepts --user, --password and --secret, but
// a service configured in System Settings already carries a Keychain reference
// for them, so the password never has to travel through this process at all.
func Start(ctx context.Context, id string) error {
	if err := validate(id); err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, scutilPath, "--nc", "start", id)
	if err != nil {
		return fmt.Errorf("start macOS VPN service: %s", failure(res, err))
	}
	return nil
}

// Stop asks macOS to tear a service down.
func Stop(ctx context.Context, id string) error {
	if err := validate(id); err != nil {
		return err
	}
	res, err := sysexec.Run(ctx, scutilPath, "--nc", "stop", id)
	if err != nil {
		return fmt.Errorf("stop macOS VPN service: %s", failure(res, err))
	}
	return nil
}

func validate(id string) error {
	if !serviceID.MatchString(id) {
		return fmt.Errorf("VPN service identifier %q is not valid", id)
	}
	return nil
}

func failure(res sysexec.Result, err error) string {
	if combined := strings.TrimSpace(res.Combined()); combined != "" {
		return combined
	}
	return err.Error()
}
