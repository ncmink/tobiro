//go:build !darwin

package keychain

import (
	"fmt"

	"github.com/zalando/go-keyring"

	"tobiro/internal/security"
)

func platformSet(service, account string, write security.SecretWrite) error {
	if err := keyring.Set(service, account, write.Value); err != nil {
		return fmt.Errorf("store secret: %w", err)
	}
	return nil
}
