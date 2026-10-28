package domain

import (
	"strings"
	"testing"
)

func sshInput(mutate func(*ProfileInput)) ProfileInput {
	in := ProfileInput{
		Name: "SMOI Bastion",
		Kind: KindSSH,
		Host: "172.16.20.65",
		SSH: SSHSettings{
			User: "server1",
			Forwards: []PortForward{
				{Name: "Kubernetes API", LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443},
			},
		},
	}
	if mutate != nil {
		mutate(&in)
	}
	return in
}

func TestSSHProfileDefaultsToPort22(t *testing.T) {
	in := sshInput(nil).Normalise()
	if in.Port != DefaultSSHPort {
		t.Fatalf("port = %d, want %d", in.Port, DefaultSSHPort)
	}
	if in.Provider != ProviderSSH {
		t.Fatalf("provider = %q, want %q", in.Provider, ProviderSSH)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("valid SSH profile rejected: %v", err)
	}
}

// An empty kind is every profile written before SSH tunnels existed, and it has
// to keep validating as the VPN profile it is.
func TestEmptyKindStaysVPN(t *testing.T) {
	in := ProfileInput{
		Name:     "SMOI Production",
		Provider: ProviderForcepoint,
		Host:     "vpn.smoi.example",
		Port:     443,
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("VPN profile with no kind rejected: %v", err)
	}
	if got := in.Normalise().Kind; got != KindVPN {
		t.Fatalf("kind = %q, want %q", got, KindVPN)
	}
}

func TestSSHProfileValidationRejects(t *testing.T) {
	cases := []struct {
		name    string
		input   ProfileInput
		wantErr string
	}{
		{
			name:    "a login name that is not one",
			input:   sshInput(func(in *ProfileInput) { in.SSH.User = "server1; rm -rf /" }),
			wantErr: "invalid characters",
		},
		{
			name:    "a server that is not one",
			input:   sshInput(func(in *ProfileInput) { in.Host = "172.16.20.65 && curl evil" }),
			wantErr: "invalid characters",
		},
		{
			// Two listeners cannot bind the same port, so the tunnel would come
			// up with one of the two forwards silently missing.
			name: "the same local port twice",
			input: sshInput(func(in *ProfileInput) {
				in.SSH.Forwards = append(in.SSH.Forwards, PortForward{
					LocalPort: 16443, RemoteHost: "localhost", RemotePort: 8080,
				})
			}),
			wantErr: "forwarded twice",
		},
		{
			name: "a port out of range",
			input: sshInput(func(in *ProfileInput) {
				in.SSH.Forwards[0].RemotePort = 70000
			}),
			wantErr: "between 1 and 65535",
		},
		{
			name: "a remote host that is not one",
			input: sshInput(func(in *ProfileInput) {
				in.SSH.Forwards[0].RemoteHost = "local host; rm -rf /"
			}),
			wantErr: "invalid characters",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.input.Validate()
			if err == nil {
				t.Fatal("expected the profile to be rejected")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// A tunnel is named and filed first and set up afterwards, on its own page, the
// same way its accounts are. So an empty one has to save.
func TestATunnelIsSavedBeforeItIsSetUp(t *testing.T) {
	in := ProfileInput{Name: "SMOI Bastion", Kind: KindSSH}
	if err := in.Validate(); err != nil {
		t.Fatalf("an unconfigured tunnel was rejected: %v", err)
	}
}

func TestAnUnfinishedTunnelSaysWhatItIsMissing(t *testing.T) {
	cases := []struct {
		name    string
		profile VPNProfile
		wantErr string
	}{
		{
			name:    "nothing filled in",
			profile: VPNProfile{Kind: KindSSH},
			wantErr: "no SSH server",
		},
		{
			name:    "a server but no login",
			profile: VPNProfile{Kind: KindSSH, Host: "172.16.20.65", Port: 22},
			wantErr: "login name",
		},
		{
			name: "a login with no ports yet",
			profile: VPNProfile{
				Kind: KindSSH, Host: "172.16.20.65", Port: 22,
				SSH: SSHSettings{User: "server1"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.ReadyToConnect()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected ready to connect: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an unfinished tunnel to refuse to connect")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

func TestAFinishedTunnelIsReady(t *testing.T) {
	profile := VPNProfile{
		Kind: KindSSH, Host: "172.16.20.65", Port: 22,
		SSH: SSHSettings{
			User:     "server1",
			Forwards: []PortForward{{LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443}},
		},
	}
	if err := profile.ReadyToConnect(); err != nil {
		t.Fatalf("a configured tunnel was refused: %v", err)
	}
	// A VPN has no SSH half to be missing, and must never be held up by one.
	if err := (VPNProfile{Provider: ProviderForcepoint}).ReadyToConnect(); err != nil {
		t.Fatalf("a VPN was refused: %v", err)
	}
}

// A missing remote host is the normal state of a half-typed form, and the far
// side's own loopback is what the field is for.
func TestForwardDefaultsToLoopbackOnTheFarSide(t *testing.T) {
	in := sshInput(func(in *ProfileInput) { in.SSH.Forwards[0].RemoteHost = "  " }).Normalise()
	if got := in.SSH.Forwards[0].RemoteHost; got != "localhost" {
		t.Fatalf("remote host = %q, want localhost", got)
	}
}

func TestSSHFieldsAreDroppedFromAVPNProfile(t *testing.T) {
	in := ProfileInput{
		Name:     "SMOI Production",
		Kind:     KindVPN,
		Provider: ProviderForcepoint,
		Host:     "vpn.smoi.example",
		Port:     443,
		SSH:      SSHSettings{User: "server1"},
	}.Normalise()

	if in.SSH.User != "" {
		t.Fatalf("a VPN profile kept SSH settings: %+v", in.SSH)
	}
}
