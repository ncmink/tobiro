package main

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tobiro/internal/hotkeyspec"
)

func buildApplicationMenu(app *App) *application.Menu {
	menu := application.NewMenu()

	appMenu := menu.AddSubmenu("Tobiro")
	appMenu.AddRole(application.About)
	appMenu.AddSeparator()
	appMenu.Add("Check for Updates…").OnClick(func(_ *application.Context) {
		go func() { _ = app.CheckForUpdate() }()
	})
	appMenu.AddSeparator()
	appMenu.AddRole(application.ServicesMenu)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Hide)
	appMenu.AddRole(application.HideOthers)
	appMenu.AddRole(application.UnHide)
	appMenu.AddSeparator()
	appMenu.AddRole(application.Quit)

	menu.AddRole(application.EditMenu)

	conn := menu.AddSubmenu("Connection")
	if runtime.GOOS == "darwin" {
		combo, _ := hotkeyspec.Parse(app.fillHotkeyStorage())
		item := conn.Add("Fill VPN login")
		if accel := menuAccelerator(combo); accel != "" {
			item.SetAccelerator(accel)
		}
		item.OnClick(func(_ *application.Context) {
			go func() { _ = app.FillNativeLoginHotkey("") }()
		})
	}

	menu.AddRole(application.WindowMenu)
	menu.AddRole(application.HelpMenu)

	return menu
}

func (a *App) refreshApplicationMenu() {
	wails := application.Get()
	if wails == nil {
		return
	}
	wails.Menu.SetApplicationMenu(buildApplicationMenu(a))
	if menu := wails.Menu.GetApplicationMenu(); menu != nil {
		menu.Update()
	}
}

func menuAccelerator(c hotkeyspec.Combo) string {
	if c.Key == "" {
		return ""
	}
	parts := make([]string, 0, len(c.Modifiers)+1)
	for _, m := range c.Modifiers {
		switch m {
		case "cmd":
			parts = append(parts, "Cmd")
		case "ctrl":
			parts = append(parts, "Ctrl")
		case "option":
			parts = append(parts, "Alt")
		case "shift":
			parts = append(parts, "Shift")
		}
	}
	parts = append(parts, c.Key)
	return stringsJoinAccelerator(parts)
}

func stringsJoinAccelerator(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "+" + p
	}
	return out
}
