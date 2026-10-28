package sshconfig

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleConfig = `
Host moi-ha-5
    HostName 172.16.20.69
    User server5
    ServerAliveInterval 60

Host moi-control-1
    HostName 172.16.20.65
    User server1
    ServerAliveInterval 60

Host moi-worker-2
    HostName 172.16.20.66
    User server2
    ServerAliveInterval 60

Host moi-worker-3
    HostName 172.16.20.67
    User server3
    ServerAliveInterval 60

Host moi-st-4
    HostName 172.16.20.68
    User server4
    ServerAliveInterval 60
`

func TestParseSampleMOIHosts(t *testing.T) {
	got := Parse([]byte(sampleConfig))
	if len(got) != 5 {
		t.Fatalf("hosts = %d, want 5: %+v", len(got), got)
	}
	expect := map[string]struct {
		host string
		user string
	}{
		"moi-ha-5":      {"172.16.20.69", "server5"},
		"moi-control-1": {"172.16.20.65", "server1"},
		"moi-worker-2":  {"172.16.20.66", "server2"},
		"moi-worker-3":  {"172.16.20.67", "server3"},
		"moi-st-4":      {"172.16.20.68", "server4"},
	}
	for _, h := range got {
		want, ok := expect[h.Alias]
		if !ok {
			t.Errorf("unexpected host %q", h.Alias)
			continue
		}
		if h.HostName != want.host || h.User != want.user {
			t.Errorf("%q = %+v, want host=%s user=%s", h.Alias, h, want.host, want.user)
		}
		if h.Port != 22 {
			t.Errorf("%q port = %d, want 22", h.Alias, h.Port)
		}
	}
}

func TestParseSkipsPatterns(t *testing.T) {
	raw := `
Host *
    User nobody

Host *.example.com
    User wild

Host good
    HostName 10.0.0.1
    User alice
`
	got := Parse([]byte(raw))
	if len(got) != 1 || got[0].Alias != "good" {
		t.Fatalf("got %+v, want only good", got)
	}
}

func TestParseLocalForward(t *testing.T) {
	raw := `
Host jump
    HostName 10.0.0.1
    User admin
    LocalForward 16443 localhost:6443
`
	got := Parse([]byte(raw))
	if len(got) != 1 {
		t.Fatalf("hosts = %d", len(got))
	}
	if len(got[0].Forwards) != 1 {
		t.Fatalf("forwards = %+v", got[0].Forwards)
	}
	f := got[0].Forwards[0]
	if f.LocalPort != 16443 || f.RemoteHost != "localhost" || f.RemotePort != 6443 {
		t.Fatalf("forward = %+v", f)
	}
}

func TestParseMultipleAliasesOnOneHostLine(t *testing.T) {
	raw := `
Host a b
    HostName shared.example
    User shared
`
	got := Parse([]byte(raw))
	if len(got) != 2 {
		t.Fatalf("got %d hosts", len(got))
	}
	for _, h := range got {
		if h.HostName != "shared.example" || h.User != "shared" {
			t.Fatalf("unexpected %+v", h)
		}
	}
}

func TestParseFileFollowsInclude(t *testing.T) {
	dir := t.TempDir()
	included := filepath.Join(dir, "hosts.conf")
	if err := os.WriteFile(included, []byte(`
Host from-include
    HostName 10.1.1.1
    User inc
`), 0o600); err != nil {
		t.Fatal(err)
	}
	mainCfg := filepath.Join(dir, "config")
	if err := os.WriteFile(mainCfg, []byte(`
Host local-only
    HostName 10.2.2.2
    User loc

Include hosts.conf
`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseFile(mainCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("hosts = %d, want 2: %+v", len(got), got)
	}
	byAlias := map[string]Host{}
	for _, h := range got {
		byAlias[h.Alias] = h
	}
	if h := byAlias["from-include"]; h.HostName != "10.1.1.1" || h.User != "inc" {
		t.Fatalf("from-include = %+v", h)
	}
	if h := byAlias["local-only"]; h.HostName != "10.2.2.2" || h.User != "loc" {
		t.Fatalf("local-only = %+v", h)
	}
}

func TestParseFileIncludeGlob(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.conf": "Host alpha\n    HostName 1.1.1.1\n    User a\n",
		"b.conf": "Host beta\n    HostName 2.2.2.2\n    User b\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mainCfg := filepath.Join(dir, "config")
	if err := os.WriteFile(mainCfg, []byte("Include *.conf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseFile(mainCfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("hosts = %d, want 2: %+v", len(got), got)
	}
}

func TestParseFileIncludeCycleDoesNotHang(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("Include b\nHost ok\n    HostName 1.1.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("Include a\nHost bad\n    HostName 2.2.2.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseFile(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d hosts, want 2 (included file still parsed once): %+v", len(got), got)
	}
}
