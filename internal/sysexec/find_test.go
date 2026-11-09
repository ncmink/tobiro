package sysexec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindFileIsCaseInsensitiveAndBounded(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "NGFW VPN Client", "bin")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nested, "Sgvpn.exe")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if got := findFile(root, "sgvpn.exe", 3); got != want {
		t.Fatalf("findFile = %q, want %q", got, want)
	}
	if got := findFile(root, "sgvpn.exe", 1); got != "" {
		t.Fatalf("findFile beyond depth = %q, want empty", got)
	}
}
