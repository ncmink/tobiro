//go:build darwin

package keychain

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"tobiro/internal/security"
)

const base64EncodingPrefix = "go-keyring-base64:"

const execPathKeychain = "/usr/bin/security"

// platformSet stores a generic password with an optional Keychain comment.
//
// Arguments are passed directly to security(1). The old go-keyring interactive
// mode breaks when the comment contains a newline — security treats the second
// line as a new command ("SSH" in "SSH tunnel: …").
func platformSet(service, account string, write security.SecretWrite) error {
	password := base64EncodingPrefix + base64.StdEncoding.EncodeToString([]byte(write.Value))

	args := []string{
		"add-generic-password", "-U",
		"-s", service,
		"-a", account,
		"-w", password,
	}
	if write.Comment != "" {
		args = append(args, "-j", write.Comment)
	}

	out, err := exec.Command(execPathKeychain, args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, detail)
	}
	return nil
}
