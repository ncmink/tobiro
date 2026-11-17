package openvpn

import (
	"sync"

	"tobiro/internal/domain"
)

var (
	openedMu sync.Mutex
	openedID string
)

func rememberOpened(profile domain.VPNProfile) {
	openedMu.Lock()
	openedID = profile.ID
	openedMu.Unlock()
}

func lastOpenedProfileID() string {
	openedMu.Lock()
	defer openedMu.Unlock()
	return openedID
}