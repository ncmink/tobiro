package forticlient

import (
	"context"
	"testing"

	"tobiro/internal/domain"
)

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

func TestFillNativeLoginSkipsEmptyCredential(t *testing.T) {
	p := New()
	if err := p.FillNativeLogin(context.Background(), domain.VPNProfile{}, domain.Credential{}); err != nil {
		t.Fatalf("empty credential should be a no-op: %v", err)
	}
}
