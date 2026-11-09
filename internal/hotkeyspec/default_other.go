//go:build !darwin

package hotkeyspec

// DefaultStorage is the persisted shortcut when FillHotkey is empty. Ctrl+I
// alone is italics in most Windows apps, and a global hotkey would steal it.
const DefaultStorage = "ctrl+shift+i"
