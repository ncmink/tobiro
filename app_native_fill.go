package main

import (
	"errors"
	"fmt"

	"tobiro/internal/domain"
	"tobiro/internal/store"
	"tobiro/internal/vpn/nativefill"
)

// setNativeFillProfile remembers which connection ⌘I should fill for.
func (a *App) setNativeFillProfile(profileID string) {
	if profileID == "" {
		return
	}
	a.fillTargetMu.Lock()
	a.fillTargetProfileID = profileID
	a.fillTargetMu.Unlock()

	if err := a.store.Update(func(d *store.Data) error {
		d.LastNativeFillProfileID = profileID
		return nil
	}); err != nil {
		a.log.Warn("Could not remember ⌘I connection: %v", err)
	}
}

func (a *App) restoreNativeFillProfile() {
	a.store.Read(func(d *store.Data) {
		if d.LastNativeFillProfileID == "" {
			return
		}
		a.fillTargetMu.Lock()
		a.fillTargetProfileID = d.LastNativeFillProfileID
		a.fillTargetMu.Unlock()
	})
}

func (a *App) nativeFillProfileID() string {
	a.fillTargetMu.Lock()
	defer a.fillTargetMu.Unlock()
	return a.fillTargetProfileID
}

// SetNativeFillProfile tells Tobiro which connection ⌘I applies to (for example
// the connection detail page the user is viewing).
func (a *App) SetNativeFillProfile(profileID string) {
	a.setNativeFillProfile(profileID)
}

// FillNativeLoginHotkey runs fill for the remembered profile using the suggested
// account. Pass an empty profileID to use the last target.
func (a *App) FillNativeLoginHotkey(profileID string) error {
	err := a.fillNativeLoginHotkey(profileID)
	if err != nil {
		a.log.Warn("Fill login: %v", err)
	}
	a.notifyFillLoginResult(err)
	return a.describe(err)
}

func (a *App) fillNativeLoginHotkey(profileID string) error {
	if profileID == "" {
		remembered := a.nativeFillProfileID()
		front := nativefill.FrontmostProcess(a.ctx)
		profileID = a.vpn.ProfileForNativeProcess(front, remembered)
		if profileID == "" {
			profileID = remembered
		}
		a.log.Info("Fill login hotkey: front app=%q connection=%s", front, profileID)
	}
	if profileID == "" {
		return fmt.Errorf("choose a connection in Tobiro first (open it or use Open client), then press ⌘I in the VPN login window")
	}
	if err := a.requireVendorClient(profileID, "fill login for"); err != nil {
		return err
	}
	credentialID, err := a.pickFillCredential(profileID)
	if err != nil {
		return err
	}
	a.setNativeFillProfile(profileID)
	return a.vpn.FillNativeLogin(a.ctx, profileID, credentialID)
}

func (a *App) notifyFillLoginResult(err error) {
	if a.bridge == nil {
		return
	}
	if err != nil {
		a.bridge.present()
	}
	if err == nil {
		a.bridge.Notify(domain.EventFillLoginResult, FillLoginResultView{
			OK:      true,
			Message: "Filled username and password in the vendor login window.",
		})
		return
	}
	a.bridge.Notify(domain.EventFillLoginResult, FillLoginResultView{
		OK:      false,
		Message: fillLoginErrorMessage(err),
	})
}

func fillLoginErrorMessage(err error) string {
	switch {
	case errors.Is(err, nativefill.ErrNoFocus):
		return "Click a username or password field in the VPN client, then press ⌘I again. Tobiro keeps the connection even when another app was in front."
	case errors.Is(err, nativefill.ErrNotEditable):
		return "The VPN client focus is not in a login field — click inside the username or password box, then press ⌘I."
	case errors.Is(err, nativefill.ErrNoVendorApp):
		return "Open the VPN client for this connection, click a login field, then press ⌘I."
	default:
		return err.Error()
	}
}

func (a *App) pickFillCredential(profileID string) (string, error) {
	view, err := a.GetConnection(profileID)
	if err != nil {
		return "", err
	}
	if !view.Capabilities.CanFillNativeLogin {
		return "", fmt.Errorf("%s does not support login fill from Tobiro", view.Profile.Name)
	}
	var eligible []string
	for _, c := range view.Credentials {
		if c.Username != "" || c.HasPassword {
			eligible = append(eligible, c.ID)
		}
	}
	if len(eligible) == 0 {
		return "", fmt.Errorf("save an account on %s before using ⌘I", view.Profile.Name)
	}
	if view.SuggestedCredentialID != "" {
		for _, id := range eligible {
			if id == view.SuggestedCredentialID {
				return id, nil
			}
		}
	}
	if len(eligible) == 1 {
		return eligible[0], nil
	}
	return eligible[0], nil
}
