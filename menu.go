package main

import (
	"context"
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tobiro/internal/hotkeyspec"
)

func applicationMenu(app *App) *menu.Menu {
	m := menu.NewMenu()
	m.Append(menu.AppMenu())
	m.Append(menu.EditMenu())

	conn := m.AddSubmenu("Connection")
	if runtime.GOOS == "darwin" {
		combo, _ := hotkeyspec.Parse(app.fillHotkeyStorage())
		conn.AddText("Fill VPN login", wailsAccelerator(combo), func(_ *menu.CallbackData) {
			go func() { _ = app.FillNativeLoginHotkey("") }()
		})
	}

	return m
}

func refreshApplicationMenu(ctx context.Context, app *App) {
	wailsruntime.MenuSetApplicationMenu(ctx, applicationMenu(app))
	wailsruntime.MenuUpdateApplicationMenu(ctx)
}

func wailsAccelerator(c hotkeyspec.Combo) *keys.Accelerator {
	wm := make([]keys.Modifier, 0, len(c.Modifiers))
	for _, m := range c.Modifiers {
		switch m {
		case "cmd":
			wm = append(wm, keys.CmdOrCtrlKey)
		case "ctrl":
			wm = append(wm, keys.ControlKey)
		case "option":
			wm = append(wm, keys.OptionOrAltKey)
		case "shift":
			wm = append(wm, keys.ShiftKey)
		}
	}
	if len(wm) == 0 {
		return keys.CmdOrCtrl(c.Key)
	}
	if len(wm) == 1 {
		return &keys.Accelerator{Key: c.Key, Modifiers: wm}
	}
	return keys.Combo(c.Key, wm[0], wm[1], wm[2:]...)
}
