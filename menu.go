package main

import (
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

func applicationMenu(app *App) *menu.Menu {
	m := menu.NewMenu()
	m.Append(menu.AppMenu())
	m.Append(menu.EditMenu())

	conn := m.AddSubmenu("Connection")
	conn.AddText("Fill VPN login", keys.CmdOrCtrl("i"), func(_ *menu.CallbackData) {
		go func() { _ = app.FillNativeLoginHotkey("") }()
	})

	return m
}
