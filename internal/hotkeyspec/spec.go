package hotkeyspec

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// DefaultStorage is the persisted shortcut when FillHotkey is empty.
const DefaultStorage = "cmd+i"

// Combo is a global shortcut: one or more modifiers plus a single key.
type Combo struct {
	Modifiers []string
	Key       string
}

var (
	ErrInvalidCombo   = errors.New("shortcut must include Command, Control, or Option plus one letter, number, or function key")
	ErrReservedCombo   = errors.New("that shortcut is reserved by macOS or Tobiro")
	ErrRegisterFailed  = errors.New("could not register the shortcut — another app may already use it")
	ErrUnknownKey     = errors.New("unsupported key")
	ErrUnknownModifier = errors.New("unsupported modifier")
)

var validModifiers = map[string]struct{}{
	"cmd": {}, "ctrl": {}, "option": {}, "shift": {},
}

// Parse reads a stored shortcut such as "cmd+shift+l".
func Parse(raw string) (Combo, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return Parse(DefaultStorage)
	}
	parts := strings.Split(raw, "+")
	if len(parts) < 2 {
		return Combo{}, ErrInvalidCombo
	}
	key := parts[len(parts)-1]
	if key == "" {
		return Combo{}, ErrInvalidCombo
	}
	modSet := map[string]struct{}{}
	for _, p := range parts[:len(parts)-1] {
		p = strings.TrimSpace(p)
		if p == "" {
			return Combo{}, ErrInvalidCombo
		}
		if _, ok := validModifiers[p]; !ok {
			return Combo{}, fmt.Errorf("%w: %q", ErrUnknownModifier, p)
		}
		modSet[p] = struct{}{}
	}
	if len(modSet) == 0 {
		return Combo{}, ErrInvalidCombo
	}
	if _, hasPrimary := modSet["cmd"]; !hasPrimary {
		if _, ok := modSet["ctrl"]; !ok {
			if _, ok := modSet["option"]; !ok {
				return Combo{}, ErrInvalidCombo
			}
		}
	}
	if err := validateKeyToken(key); err != nil {
		return Combo{}, err
	}
	mods := make([]string, 0, len(modSet))
	for m := range modSet {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	c := Combo{Modifiers: mods, Key: key}
	if err := c.ValidateReserved(); err != nil {
		return Combo{}, err
	}
	return c, nil
}

func validateKeyToken(key string) error {
	if len(key) == 1 && key[0] >= 'a' && key[0] <= 'z' {
		return nil
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		return nil
	}
	if strings.HasPrefix(key, "f") {
		n := key[1:]
		if n >= "1" && n <= "12" && len(n) <= 2 {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrUnknownKey, key)
}

// Canonical returns the storage form.
func (c Combo) Canonical() string {
	if len(c.Modifiers) == 0 && c.Key == "" {
		return DefaultStorage
	}
	mods := append([]string(nil), c.Modifiers...)
	sort.Strings(mods)
	return strings.Join(append(mods, c.Key), "+")
}

// ValidateReserved rejects shortcuts that would fight the OS or Tobiro.
func (c Combo) ValidateReserved() error {
	hasCmd := false
	for _, m := range c.Modifiers {
		if m == "cmd" {
			hasCmd = true
			break
		}
	}
	if !hasCmd {
		return nil
	}
	switch c.Key {
	case "q", "w", "c", "v", "x", "a", "z", "tab", "space":
		return ErrReservedCombo
	}
	return nil
}

// FormatDisplay renders a human label (macOS-style symbols when darwinDisplay is true).
func FormatDisplay(c Combo, darwinDisplay bool) string {
	if len(c.Modifiers) == 0 && c.Key == "" {
		c, _ = Parse(DefaultStorage)
	}
	parts := make([]string, 0, len(c.Modifiers)+1)
	for _, m := range c.Modifiers {
		if darwinDisplay {
			switch m {
			case "cmd":
				parts = append(parts, "⌘")
			case "shift":
				parts = append(parts, "⇧")
			case "option":
				parts = append(parts, "⌥")
			case "ctrl":
				parts = append(parts, "⌃")
			default:
				parts = append(parts, m)
			}
		} else {
			switch m {
			case "cmd":
				parts = append(parts, "Cmd")
			case "option":
				parts = append(parts, "Option")
			case "ctrl":
				parts = append(parts, "Ctrl")
			case "shift":
				parts = append(parts, "Shift")
			}
		}
	}
	keyLabel := strings.ToUpper(c.Key)
	if strings.HasPrefix(c.Key, "f") {
		keyLabel = strings.ToUpper(c.Key)
	}
	parts = append(parts, keyLabel)
	if darwinDisplay {
		return strings.Join(parts, "")
	}
	return strings.Join(parts, "+")
}

// FromEvent builds a combo from a captured key event (modifiers + key code name).
func FromEvent(modifiers map[string]bool, key string) (Combo, error) {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return Combo{}, ErrInvalidCombo
	}
	if err := validateKeyToken(key); err != nil {
		return Combo{}, err
	}
	mods := make([]string, 0, 4)
	for _, name := range []string{"cmd", "ctrl", "option", "shift"} {
		if modifiers[name] {
			mods = append(mods, name)
		}
	}
	if len(mods) == 0 {
		return Combo{}, ErrInvalidCombo
	}
	c := Combo{Modifiers: mods, Key: key}
	sort.Strings(c.Modifiers)
	if err := c.ValidateReserved(); err != nil {
		return Combo{}, err
	}
	return c, nil
}
