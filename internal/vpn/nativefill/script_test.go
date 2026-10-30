package nativefill

import (
	"testing"
)

func TestQuoteAppleScript(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`ming`, `character id {109, 105, 110, 103}`},
		{`say "hi"`, `character id {115, 97, 121, 32, 34, 104, 105, 34}`},
		{`a\b`, `character id {97, 92, 98}`},
		{"", `""`},
	}
	for _, tc := range cases {
		if got := QuoteAppleScript(tc.in); got != tc.want {
			t.Fatalf("QuoteAppleScript(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestValidateFillValueRejectsControlChars(t *testing.T) {
	if err := ValidateFillValue("username", "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateFillValue("password", "line\nbreak"); err == nil {
		t.Fatal("expected newline in password to be rejected")
	}
}

func TestIsAssistiveAccessError(t *testing.T) {
	if !IsAssistiveAccessError("System Events got an error: osascript is not allowed assistive access.") {
		t.Fatal("expected assistive-access detection")
	}
	if IsAssistiveAccessError("execution error: Can’t get window 1") {
		t.Fatal("missing window is not an access denial")
	}
}
