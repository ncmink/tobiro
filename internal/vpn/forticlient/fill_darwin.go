//go:build darwin

package forticlient

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	"tobiro/internal/domain"
)

const (
	fillAttemptTimeout = 20 * time.Second
	osascriptPath      = "/usr/bin/osascript"
)

// FillNativeLogin types the stored username and password into FortiClient.
// It never clicks Connect. Paste is not used: FortiClient rejects it.
func (p *Provider) FillNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return nil
	}
	if err := validateFillValue("username", credential.Username); err != nil {
		return err
	}
	if err := validateFillValue("password", credential.Secret); err != nil {
		return err
	}

	result, stderr, err := runFillScript(ctx, credential.Username, credential.Secret)
	if err != nil {
		if isAssistiveAccessError(stderr + err.Error()) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
	switch strings.TrimSpace(result) {
	case "filled":
		return nil
	case "waiting":
		return errFillWaiting
	default:
		if isAssistiveAccessError(stderr) {
			return domain.ErrAssistiveAccess
		}
		return errFillWaiting
	}
}

func runFillScript(ctx context.Context, username, password string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, fillAttemptTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, osascriptPath, "-")
	cmd.Stdin = strings.NewReader(buildFillScript(username, password))
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return out.String(), errBuf.String(), err
}
