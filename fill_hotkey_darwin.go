//go:build darwin

package main

import (
	"golang.design/x/hotkey"
)

// startFillHotkey registers a system-wide ⌘I so fill works while the vendor
// login field is focused. Requires the same Accessibility permission as fill.
func (a *App) startFillHotkey() {
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModCmd}, hotkey.KeyI)
	if err := hk.Register(); err != nil {
		a.log.Warn("Global ⌘I fill hotkey unavailable (grant Accessibility in System Settings): %v", err)
		return
	}
	a.fillHotkeyStop = func() { _ = hk.Unregister() }
	go func() {
		for range hk.Keydown() {
			_ = a.FillNativeLoginHotkey("")
		}
	}()
}

func (a *App) stopFillHotkey() {
	if a.fillHotkeyStop != nil {
		a.fillHotkeyStop()
		a.fillHotkeyStop = nil
	}
}
