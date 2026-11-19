package domain

import (
	"fmt"
	"testing"
)

func TestValidateTags_reservedName(t *testing.T) {
	err := ValidateTags([]Tag{{Key: NameTagKey, Value: "x"}})
	if err == nil {
		t.Fatal("expected error for Name key")
	}
}

func TestValidateTags_limits(t *testing.T) {
	var tags []Tag
	for i := 0; i < 51; i++ {
		tags = append(tags, Tag{Key: fmtKey(i), Value: "v"})
	}
	if err := ValidateTags(tags); err == nil {
		t.Fatal("expected too many tags error")
	}
}

func fmtKey(i int) string {
	return fmt.Sprintf("key%d", i)
}

func TestNormaliseTags_dropsEmptyKey(t *testing.T) {
	got := NormaliseTags([]Tag{{Key: "  ", Value: "x"}, {Key: "env", Value: " prod "}})
	if len(got) != 1 || got[0].Key != "env" || got[0].Value != "prod" {
		t.Fatalf("got %#v", got)
	}
}

func TestMatchTagQuery(t *testing.T) {
	name := "Corp VPN"
	tags := []Tag{{Key: "env", Value: "prod"}, {Key: "team", Value: "netops"}}

	tests := []struct {
		q    string
		want bool
	}{
		{"env=prod", true},
		{"env=PRO", true},
		{"env=staging", false},
		{"env=", true},
		{"missing=", false},
		{"Name=corp", true},
		{"Name=", true},
		{"prod", true},
		{"netops", true},
		{"Env", true},
	}
	for _, tc := range tests {
		if got := MatchTagQuery(name, tags, tc.q); got != tc.want {
			t.Errorf("query %q: got %v want %v", tc.q, got, tc.want)
		}
	}
}