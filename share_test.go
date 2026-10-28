package main

import (
	"testing"

	"tobiro/internal/domain"
)

func TestFilterProfilesForExportSkipsHiddenGroups(t *testing.T) {
	hidden := map[string]bool{
		"Acme": true,
	}
	groupHidden := func(name string) bool { return hidden[name] }

	profiles := []domain.VPNProfile{
		{ID: "v1", Name: "Visible VPN", Provider: domain.ProviderForcepoint, Host: "a.example.com", Port: 443},
		{ID: "v2", Name: "Hidden VPN", Group: "Acme", Provider: domain.ProviderForcepoint, Host: "b.example.com", Port: 443},
		{
			ID: "t1", Name: "Tunnel on hidden VPN", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
			Host: "10.0.0.1", Port: 22, Requires: "v2",
		},
		{
			ID: "t2", Name: "Tunnel on visible VPN", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
			Host: "10.0.0.2", Port: 22, Requires: "v1",
		},
	}

	got := filterProfilesForExport(profiles, groupHidden)
	if len(got) != 2 {
		t.Fatalf("exportable = %d, want 2: %+v", len(got), got)
	}
	if got[0].ID != "v1" || got[1].ID != "t2" {
		t.Fatalf("unexpected export set: %+v", got)
	}
}

func TestFilterProfilesForExportSkipsDependentInVisibleGroupWhenParentHidden(t *testing.T) {
	groupHidden := func(name string) bool { return name == "Acme" }

	profiles := []domain.VPNProfile{
		{ID: "v2", Name: "Hidden VPN", Group: "Acme", Provider: domain.ProviderTailscale},
		{
			ID: "t1", Name: "Ungrouped tunnel", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
			Host: "10.0.0.1", Port: 22, Requires: "v2",
		},
	}

	got := filterProfilesForExport(profiles, groupHidden)
	if len(got) != 0 {
		t.Fatalf("exportable = %d, want 0: %+v", len(got), got)
	}
}
