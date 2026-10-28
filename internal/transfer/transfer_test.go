package transfer

import (
	"strings"
	"testing"

	"tobiro/internal/domain"
)

func TestMarshalOmitsSecretsAndLocalValues(t *testing.T) {
	profiles := []domain.VPNProfile{{
		ID:         "9f0c1b3e-0000-0000-0000-000000000001",
		Name:       "SMOI",
		Provider:   domain.ProviderForcepoint,
		Host:       "vpn.example.com",
		Port:       10443,
		ClientPath: "/Applications/Forcepoint.app/Contents/MacOS/Forcepoint",
	}}
	accounts := map[string][]domain.CredentialView{
		profiles[0].ID: {{
			ID:          "cred-1",
			Label:       "Ming",
			Username:    "ming",
			AuthMethod:  domain.AuthPassword,
			IsDefault:   true,
			HasPassword: true,
		}},
	}

	raw, err := Marshal(Build(profiles, func(id string) []domain.CredentialView { return accounts[id] }))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(raw)

	for _, unwanted := range []string{"secret", "password:", "Forcepoint.app", profiles[0].ID, "cred-1"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(unwanted)) {
			t.Errorf("export leaks %q:\n%s", unwanted, out)
		}
	}
	for _, wanted := range []string{"name: SMOI", "provider: forcepoint", "port: 10443", "label: Ming"} {
		if !strings.Contains(out, wanted) {
			t.Errorf("export is missing %q:\n%s", wanted, out)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	profiles := []domain.VPNProfile{
		{ID: "p1", Name: "SMOI", Provider: domain.ProviderForcepoint, Host: "vpn.example.com", Port: 10443},
		{ID: "p2", Name: "Mesh", Provider: domain.ProviderTailscale, VendorProfile: "work@customer.example.com", Notes: "no endpoint needed"},
	}
	accounts := map[string][]domain.CredentialView{
		"p1": {{Label: "Ming", Username: "ming", AuthMethod: domain.AuthPasswordOTP, IsDefault: true}},
	}

	raw, err := Marshal(Build(profiles, func(id string) []domain.CredentialView { return accounts[id] }))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Version != Version {
		t.Errorf("version = %d, want %d", doc.Version, Version)
	}
	if len(doc.Connections) != 2 {
		t.Fatalf("connections = %d, want 2", len(doc.Connections))
	}

	got := doc.Connections[0].ProfileInput()
	if got.Name != "SMOI" || got.Provider != domain.ProviderForcepoint || got.Port != 10443 {
		t.Errorf("profile input = %+v", got)
	}
	if got.ClientPath != "" {
		t.Errorf("client path must not survive an export: %q", got.ClientPath)
	}

	mesh := doc.Connections[1].ProfileInput()
	if mesh.Provider != domain.ProviderTailscale || mesh.VendorProfile != "work@customer.example.com" {
		t.Errorf("tailscale profile input = %+v", mesh)
	}

	acct := doc.Connections[0].Accounts[0].CredentialInput()
	if acct.Label != "Ming" || acct.AuthMethod != domain.AuthPasswordOTP || !acct.IsDefault {
		t.Errorf("credential input = %+v", acct)
	}
	if acct.Password != "" {
		t.Error("imported account must have no password")
	}
}

func TestARequirementTravelsAsAName(t *testing.T) {
	profiles := []domain.VPNProfile{
		{ID: "p1", Name: "SMOI Production VPN", Provider: domain.ProviderForcepoint, Host: "vpn.example.com", Port: 10443},
		{
			ID: "p2", Name: "SMOI bastion", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
			Host: "172.16.20.65", Port: 22, Requires: "p1",
			SSH: domain.SSHSettings{
				User:     "server1",
				Forwards: []domain.PortForward{{LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443}},
			},
		},
	}

	raw, err := Marshal(Build(profiles, nil))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out := string(raw)

	// The recipient's copy of the VPN has an identifier of its own, so the
	// order can only be described by name.
	if !strings.Contains(out, "requires: SMOI Production VPN") {
		t.Fatalf("expected the requirement to be named:\n%s", out)
	}
	if strings.Contains(out, "requires: p1") {
		t.Fatalf("an internal identifier escaped into the file:\n%s", out)
	}

	doc, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Connections[1].Requires != "SMOI Production VPN" {
		t.Fatalf("requires = %q", doc.Connections[1].Requires)
	}
}

func TestARequirementOnSomethingNotBeingExportedIsDropped(t *testing.T) {
	// Only the tunnel is being shared. Naming a VPN that is not in the file
	// would describe an order the recipient cannot follow.
	profiles := []domain.VPNProfile{{
		ID: "p2", Name: "SMOI bastion", Kind: domain.KindSSH, Provider: domain.ProviderSSH,
		Host: "172.16.20.65", Port: 22, Requires: "p1",
		SSH: domain.SSHSettings{
			User:     "server1",
			Forwards: []domain.PortForward{{LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443}},
		},
	}}

	doc := Build(profiles, nil)
	if doc.Connections[0].Requires != "" {
		t.Fatalf("requires = %q, want it left out", doc.Connections[0].Requires)
	}
}

func TestParseRejectsBadDocuments(t *testing.T) {
	cases := map[string]string{
		"empty file":       "",
		"no version":       "connections:\n  - name: A\n    provider: forcepoint\n    host: a.example.com\n    port: 443\n",
		"future schema":    "version: 99\nconnections:\n  - name: A\n    provider: forcepoint\n",
		"no connections":   "version: 1\nconnections: []\n",
		"unknown provider": "version: 1\nconnections:\n  - name: A\n    provider: pulse\n",
		"missing host":     "version: 1\nconnections:\n  - name: A\n    provider: forcepoint\n",
		"unnamed":          "version: 1\nconnections:\n  - provider: forcepoint\n",
		"unknown key":      "version: 1\nconnections:\n  - name: A\n    provider: forcepoint\n    hosts: a.example.com\n    port: 443\n",
		"account no label": "version: 1\nconnections:\n  - name: A\n    provider: forcepoint\n    host: a.example.com\n    port: 443\n    accounts:\n      - username: ming\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestParseAcceptsHandWrittenMinimalFile(t *testing.T) {
	raw := `version: 1
connections:
  - name: Customer A
    provider: forcepoint
    host: vpn.customer-a.com
    port: 10443
    accounts:
      - label: My account
`
	doc, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Connections) != 1 || len(doc.Connections[0].Accounts) != 1 {
		t.Fatalf("unexpected document: %+v", doc)
	}
}
