//go:build darwin

package forcepoint

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestForcepointFillScriptCompiles(t *testing.T) {
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("osacompile not available")
	}
	script := buildFillScript("ming", `p"ass\word`)
	if strings.Contains(script, `p"ass\word`) {
		t.Fatal("password must be encoded, not embedded raw")
	}
	cmd := exec.Command("osacompile", "-o", filepath.Join(t.TempDir(), "fill.scpt"), "-")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Forcepoint fill script does not compile: %v\n%s", err, out)
	}
}
