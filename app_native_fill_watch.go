package main

import (
	"context"
	"errors"
	"time"

	"tobiro/internal/domain"
)

const fillLoginHintPollInterval = 750 * time.Millisecond
const fillLoginHintTimeout = 45 * time.Second

// watchFillLoginHint polls the vendor login window after Open client and tells
// the UI to suggest the fill shortcut once a username field is visible.
func (a *App) watchFillLoginHint(profileID string) {
	view, err := a.GetConnection(profileID)
	if err != nil || !view.Capabilities.CanFillNativeLogin {
		return
	}

	ctx, cancel := context.WithTimeout(a.ctx, fillLoginHintTimeout)
	defer cancel()

	ticker := time.NewTicker(fillLoginHintPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := a.vpn.ProbeNativeLogin(ctx, profileID)
			if err != nil {
				if errors.Is(err, domain.ErrAssistiveAccess) {
					return
				}
				continue
			}
			a.notifyFillLoginDetected(profileID, view.Profile.Name)
			return
		}
	}
}

func (a *App) notifyFillLoginDetected(profileID, connectionName string) {
	if a.bridge == nil {
		return
	}
	a.bridge.present()
	a.bridge.Notify(domain.EventFillLoginDetected, FillLoginDetectedView{
		ProfileID:       profileID,
		ConnectionName:  connectionName,
		ShortcutDisplay: a.fillShortcutDisplay(),
	})
}
