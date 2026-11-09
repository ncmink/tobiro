//go:build darwin

package main

import (
	"fmt"

	"golang.design/x/hotkey"

	"tobiro/internal/hotkeyspec"
)

func (a *App) startFillHotkey() {
	if err := a.restartFillHotkey(a.fillHotkeyStorage()); err != nil {
		a.log.Warn("Global fill hotkey unavailable (grant Accessibility in System Settings): %v", err)
	}
}

func (a *App) restartFillHotkey(comboStr string) error {
	a.fillHotkeyMu.Lock()
	defer a.fillHotkeyMu.Unlock()

	if a.fillHotkeyStop != nil {
		a.fillHotkeyStop()
		a.fillHotkeyStop = nil
	}
	a.fillHotkeyRegisterErr = ""

	combo, err := hotkeyspec.Parse(comboStr)
	if err != nil {
		return err
	}
	mods, key, err := hotkeyspec.Bind(combo)
	if err != nil {
		return err
	}

	hk := hotkey.New(mods, key)
	if err := hk.Register(); err != nil {
		a.fillHotkeyRegisterErr = err.Error()
		return fmt.Errorf("%w: %v", hotkeyspec.ErrRegisterFailed, err)
	}

	stop := func() { _ = hk.Unregister() }
	a.fillHotkeyStop = stop
	go func() {
		for range hk.Keydown() {
			_ = a.FillNativeLoginHotkey("")
		}
	}()
	return nil
}

func (a *App) stopFillHotkey() {
	a.fillHotkeyMu.Lock()
	defer a.fillHotkeyMu.Unlock()
	if a.fillHotkeyStop != nil {
		a.fillHotkeyStop()
		a.fillHotkeyStop = nil
	}
	a.fillHotkeyRegisterErr = ""
}
