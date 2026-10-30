//go:build darwin

package hotkeyspec

import (
	"fmt"
	"strings"

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

func keyTokenToHotkey(key string) (hotkey.Key, error) {
	key = strings.ToLower(key)
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		const table = "abcdefghijklmnopqrstuvwxyz"
		idx := strings.IndexByte(table, key[0])
		keys := []hotkey.Key{
			hotkey.KeyA, hotkey.KeyB, hotkey.KeyC, hotkey.KeyD, hotkey.KeyE, hotkey.KeyF,
			hotkey.KeyG, hotkey.KeyH, hotkey.KeyI, hotkey.KeyJ, hotkey.KeyK, hotkey.KeyL,
			hotkey.KeyM, hotkey.KeyN, hotkey.KeyO, hotkey.KeyP, hotkey.KeyQ, hotkey.KeyR,
			hotkey.KeyS, hotkey.KeyT, hotkey.KeyU, hotkey.KeyV, hotkey.KeyW, hotkey.KeyX,
			hotkey.KeyY, hotkey.KeyZ,
		}
		return keys[idx], nil
	}
	switch key {
	case "0":
		return hotkey.Key0, nil
	case "1":
		return hotkey.Key1, nil
	case "2":
		return hotkey.Key2, nil
	case "3":
		return hotkey.Key3, nil
	case "4":
		return hotkey.Key4, nil
	case "5":
		return hotkey.Key5, nil
	case "6":
		return hotkey.Key6, nil
	case "7":
		return hotkey.Key7, nil
	case "8":
		return hotkey.Key8, nil
	case "9":
		return hotkey.Key9, nil
	case "f1":
		return hotkey.KeyF1, nil
	case "f2":
		return hotkey.KeyF2, nil
	case "f3":
		return hotkey.KeyF3, nil
	case "f4":
		return hotkey.KeyF4, nil
	case "f5":
		return hotkey.KeyF5, nil
	case "f6":
		return hotkey.KeyF6, nil
	case "f7":
		return hotkey.KeyF7, nil
	case "f8":
		return hotkey.KeyF8, nil
	case "f9":
		return hotkey.KeyF9, nil
	case "f10":
		return hotkey.KeyF10, nil
	case "f11":
		return hotkey.KeyF11, nil
	case "f12":
		return hotkey.KeyF12, nil
	default:
		return 0, fmt.Errorf("%w: %q", ErrUnknownKey, key)
	}
}
