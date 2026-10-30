package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"tobiro/internal/hotkeyspec"
	"tobiro/internal/store"
)

const defaultFillHotkey = hotkeyspec.DefaultStorage

func (a *App) fillHotkeyStorage() string {
	var raw string
	a.store.Read(func(d *store.Data) {
		raw = d.FillHotkey
	})
	if raw == "" {
		return defaultFillHotkey
	}
	return raw
}

func (a *App) fillHotkeyIntroSeen() bool {
	var seen bool
	a.store.Read(func(d *store.Data) {
		seen = d.FillHotkeyIntroSeen
	})
	return seen
}

// GetFillShortcut reports the global fill shortcut and registration state.
func (a *App) GetFillShortcut() FillShortcutView {
	if runtime.GOOS != "darwin" {
		return FillShortcutView{Supported: false}
	}
	comboStr := a.fillHotkeyStorage()
	combo, err := hotkeyspec.Parse(comboStr)
	if err != nil {
		combo, _ = hotkeyspec.Parse(defaultFillHotkey)
		comboStr = defaultFillHotkey
	}
	a.fillHotkeyMu.Lock()
	registered := a.fillHotkeyStop != nil && a.fillHotkeyRegisterErr == ""
	regErr := a.fillHotkeyRegisterErr
	a.fillHotkeyMu.Unlock()
	return FillShortcutView{
		Supported:     true,
		Combo:         comboStr,
		Display:       hotkeyspec.FormatDisplay(combo, true),
		Registered:    registered,
		RegisterError: regErr,
		IntroSeen:     a.fillHotkeyIntroSeen(),
	}
}

// SetFillShortcut changes the global fill shortcut.
func (a *App) SetFillShortcut(combo string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("custom fill shortcuts are only supported on macOS")
	}
	parsed, err := hotkeyspec.Parse(combo)
	if err != nil {
		return a.describe(err)
	}
	canonical := parsed.Canonical()
	old := a.fillHotkeyStorage()
	if err := a.restartFillHotkey(canonical); err != nil {
		_ = a.restartFillHotkey(old)
		return a.describe(err)
	}
	if err := a.store.Update(func(d *store.Data) error {
		if canonical == defaultFillHotkey {
			d.FillHotkey = ""
		} else {
			d.FillHotkey = canonical
		}
		return nil
	}); err != nil {
		return a.describe(err)
	}
	a.refreshApplicationMenu()
	return nil
}

// ResetFillShortcut restores Command+I.
func (a *App) ResetFillShortcut() error {
	return a.SetFillShortcut(defaultFillHotkey)
}

// DismissFillShortcutIntro marks the first-run dialog as seen.
func (a *App) DismissFillShortcutIntro() {
	_ = a.store.Update(func(d *store.Data) error {
		d.FillHotkeyIntroSeen = true
		return nil
	})
}

// ShowFillShortcutIntro clears the intro flag so the dialog appears again.
func (a *App) ShowFillShortcutIntro() {
	_ = a.store.Update(func(d *store.Data) error {
		d.FillHotkeyIntroSeen = false
		return nil
	})
}

// GetFillPermissions reports macOS privacy settings relevant to fill login.
func (a *App) GetFillPermissions() FillPermissionsView {
	if runtime.GOOS != "darwin" {
		return FillPermissionsView{Supported: false}
	}
	return FillPermissionsView{
		Supported:              true,
		AccessibilityGranted: accessibilityGranted(),
		AutomationHint:         "When fill runs, macOS may ask you to allow Tobiro to control System Events. Enable it under Privacy & Security → Automation if login fill fails.",
		InputMonitoringHint:    "If the global shortcut stops working, check Privacy & Security → Input Monitoring and enable Tobiro.",
	}
}

// OpenPrivacySettings opens a macOS Privacy & Security pane.
func (a *App) OpenPrivacySettings(pane string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("privacy settings are only available on macOS")
	}
	var url string
	switch pane {
	case "accessibility":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	case "automation":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_Automation"
	case "listen":
		url = "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent"
	default:
		return fmt.Errorf("unknown privacy pane %q", pane)
	}
	return exec.Command("open", url).Run()
}

func (a *App) fillShortcutDisplay() string {
	combo, err := hotkeyspec.Parse(a.fillHotkeyStorage())
	if err != nil {
		combo, _ = hotkeyspec.Parse(defaultFillHotkey)
	}
	return hotkeyspec.FormatDisplay(combo, runtime.GOOS == "darwin")
}

func (a *App) refreshApplicationMenu() {
	if a.ctx == nil {
		return
	}
	refreshApplicationMenu(a.ctx, a)
}
