package hotkeyspec

import (
	"errors"
	"testing"
)

func TestParseDefault(t *testing.T) {
	c, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Canonical() != DefaultStorage {
		t.Fatalf("got %q", c.Canonical())
	}
}

func TestParseModifiers(t *testing.T) {
	c, err := Parse("cmd+shift+l")
	if err != nil {
		t.Fatal(err)
	}
	if c.Key != "l" {
		t.Fatalf("key=%q", c.Key)
	}
	if len(c.Modifiers) != 2 {
		t.Fatalf("mods=%v", c.Modifiers)
	}
}

func TestReservedCmdQ(t *testing.T) {
	_, err := Parse("cmd+q")
	if !errors.Is(err, ErrReservedCombo) {
		t.Fatalf("err=%v", err)
	}
}

func TestShiftOnlyRejected(t *testing.T) {
	_, err := Parse("shift+a")
	if !errors.Is(err, ErrInvalidCombo) {
		t.Fatalf("err=%v", err)
	}
}

func TestFormatDisplayDarwin(t *testing.T) {
	c, _ := Parse("cmd+i")
	if got := FormatDisplay(c, true); got != "⌘I" {
		t.Fatalf("got %q", got)
	}
}
