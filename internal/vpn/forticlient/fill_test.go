package forticlient

import (
	"context"
	"strings"
	"testing"

	"tobiro/internal/domain"
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
		if got := quoteAppleScript(tc.in); got != tc.want {
			t.Fatalf("quoteAppleScript(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestCanFill(t *testing.T) {
	if canFill(domain.Credential{}) {
		t.Fatal("empty credential should not fill")
	}
	if !canFill(domain.Credential{Username: "ming"}) {
		t.Fatal("username-only should fill")
	}
	if !canFill(domain.Credential{Secret: "secret"}) {
		t.Fatal("password-only should fill")
	}
}

func TestIsAssistiveAccessError(t *testing.T) {
	if !isAssistiveAccessError("System Events got an error: osascript is not allowed assistive access.") {
		t.Fatal("expected assistive-access detection")
	}
	if isAssistiveAccessError("execution error: Can’t get window 1") {
		t.Fatal("missing window is not an access denial")
	}
}

func TestValidateFillValueRejectsControlChars(t *testing.T) {
	if err := validateFillValue("username", "ok"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateFillValue("password", "line\nbreak"); err == nil {
		t.Fatal("expected newline in password to be rejected")
	}
}

func TestFillNativeLoginSkipsEmptyCredential(t *testing.T) {
	p := New()
	if err := p.FillNativeLogin(context.Background(), domain.VPNProfile{}, domain.Credential{}); err != nil {
		t.Fatalf("empty credential should be a no-op: %v", err)
	}
}

func TestBuildFillScriptTypesInsteadOfSubmitting(t *testing.T) {
	script := buildFillScript("ming", `p"ass\word`, false)
	for _, want := range []string{`"FortiClientAgent"`, `"FortiClient"`, "keystroke thePass", "AXSecureTextField", "AXEnhancedUserInterface"} {
		if !strings.Contains(script, want) {
			t.Fatalf("fill script missing %s", want)
		}
	}
	for _, banned := range []string{"keystroke return", "key code 36", "confirmButton", "connectButton"} {
		if strings.Contains(script, banned) {
			t.Fatalf("fill script must not submit the dialog via %s", banned)
		}
	}
	if strings.Contains(script, `p"ass\word`) {
		t.Fatal("password must be encoded, not embedded raw")
	}
}
