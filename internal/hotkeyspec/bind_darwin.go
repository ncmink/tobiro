//go:build darwin

package hotkeyspec

import (
	"fmt"

	"golang.design/x/hotkey"
)

// Bind maps a Combo to golang.design/x/hotkey values (darwin only).
func Bind(c Combo) ([]hotkey.Modifier, hotkey.Key, error) {
	mods := make([]hotkey.Modifier, 0, len(c.Modifiers))
	for _, m := range c.Modifiers {
		switch m {
		case "cmd":
			mods = append(mods, hotkey.ModCmd)
		case "ctrl":
			mods = append(mods, hotkey.ModCtrl)
		case "option":
			mods = append(mods, hotkey.ModOption)
		case "shift":
			mods = append(mods, hotkey.ModShift)
		default:
			return nil, 0, fmt.Errorf("%w: %q", ErrUnknownModifier, m)
		}
	}
	key, err := keyTokenToHotkey(c.Key)
	if err != nil {
		return nil, 0, err
	}
	return mods, key, nil
}
