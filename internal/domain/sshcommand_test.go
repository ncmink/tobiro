package domain

import (
	"strings"
	"testing"
)

// The command from the original complaint, pasted verbatim.
func TestParseSSHCommandFromARealTerminal(t *testing.T) {
	parsed, err := ParseSSHCommand(
		"ssh -N -L 16443:localhost:6443 -L 31538:localhost:31539 server1@172.16.20.65")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	profile := parsed.Profile
	if profile.Host != "172.16.20.65" {
		t.Fatalf("host = %q", profile.Host)
	}
	if profile.Port != DefaultSSHPort {
		t.Fatalf("port = %d, want the default %d", profile.Port, DefaultSSHPort)
	}
	if profile.SSH.User != "server1" {
		t.Fatalf("user = %q", profile.SSH.User)
	}
	if len(profile.SSH.Forwards) != 2 {
		t.Fatalf("forwards = %d, want 2", len(profile.SSH.Forwards))
	}

	api := profile.SSH.Forwards[0]
	if api.LocalPort != 16443 || api.RemoteHost != "localhost" || api.RemotePort != 6443 {
		t.Fatalf("first forward = %+v", api)
	}
	// The port number is the whole explanation, so the form should not make
	// someone type the label as well.
	if api.Name != "Kubernetes API" {
		t.Fatalf("first forward name = %q, want the suggested label", api.Name)
	}
}

func TestParseSSHCommandAcceptsFlagVariants(t *testing.T) {
	cases := []struct {
		name    string
		command string
		port    int
		user    string
		key     string
	}{
		{
			name:    "flag values glued to their flag",
			command: "ssh -N -L16443:localhost:6443 -p2222 server1@172.16.20.65",
			port:    2222,
			user:    "server1",
		},
		{
			name:    "a login given with -l instead of user@",
			command: "ssh -N -l server1 -L 16443:localhost:6443 172.16.20.65",
			port:    22,
			user:    "server1",
		},
		{
			name:    "an identity file",
			command: "ssh -i ~/.ssh/id_ed25519 -N -L 16443:localhost:6443 server1@172.16.20.65",
			port:    22,
			user:    "server1",
			key:     "~/.ssh/id_ed25519",
		},
		{
			name:    "options whose values must not be read as the destination",
			command: "ssh -o StrictHostKeyChecking=yes -N -L 16443:localhost:6443 server1@172.16.20.65",
			port:    22,
			user:    "server1",
		},
		{
			name:    "a bind address that is loopback anyway",
			command: "ssh -N -L 127.0.0.1:16443:localhost:6443 server1@172.16.20.65",
			port:    22,
			user:    "server1",
		},
		{
			name:    "no ssh at the front, because it was copied without it",
			command: "-N -L 16443:localhost:6443 server1@172.16.20.65",
			port:    22,
			user:    "server1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseSSHCommand(tc.command)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if parsed.Profile.Port != tc.port {
				t.Fatalf("port = %d, want %d", parsed.Profile.Port, tc.port)
			}
			if parsed.Profile.SSH.User != tc.user {
				t.Fatalf("user = %q, want %q", parsed.Profile.SSH.User, tc.user)
			}
			if parsed.PrivateKeyPath != tc.key {
				t.Fatalf("key = %q, want %q", parsed.PrivateKeyPath, tc.key)
			}
			if parsed.Profile.Host != "172.16.20.65" {
				t.Fatalf("host = %q", parsed.Profile.Host)
			}
		})
	}
}

func TestParseSSHCommandRefuses(t *testing.T) {
	cases := []struct {
		name    string
		command string
		wantErr string
	}{
		{"nothing at all", "   ", "paste an ssh command"},
		{"no forwards", "ssh server1@172.16.20.65", "at least one port"},
		{"no destination", "ssh -N -L 16443:localhost:6443", "no server"},
		{"a remote forward", "ssh -N -R 8080:localhost:80 server1@host", "not supported"},
		{"a SOCKS proxy", "ssh -N -D 1080 server1@host", "not supported"},
		{"a jump host", "ssh -J bastion -N -L 16443:localhost:6443 server1@host", "not supported"},
		{
			// Forwarding a customer's port to the whole café wifi is not
			// something to inherit silently from a pasted command.
			name:    "a forward bound to every interface",
			command: "ssh -N -L 0.0.0.0:16443:localhost:6443 server1@host",
			wantErr: "only forwards to 127.0.0.1",
		},
		{
			name:    "a malformed forward",
			command: "ssh -N -L 16443:6443 server1@host",
			wantErr: "localPort:host:remotePort",
		},
		{
			name:    "a command to run on the server",
			command: "ssh -L 16443:localhost:6443 server1@host uptime",
			wantErr: "only a plain port-forwarding command",
		},
		{
			name:    "a flag with nothing after it",
			command: "ssh -N -L 16443:localhost:6443 server1@host -p",
			wantErr: "missing its value",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSSHCommand(tc.command)
			if err == nil {
				t.Fatal("expected the command to be refused")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// Whatever the parser accepts must also survive being saved, or the paste field
// would fill a form that the Save button then rejects.
func TestParsedCommandPassesProfileValidation(t *testing.T) {
	parsed, err := ParseSSHCommand(
		"ssh -N -L 16443:localhost:6443 -L 31538:localhost:31539 server1@172.16.20.65")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	in := parsed.Profile
	in.Name = "SMOI Bastion"
	if err := in.Validate(); err != nil {
		t.Fatalf("a parsed command did not validate as a profile: %v", err)
	}
}
