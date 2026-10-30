//go:build darwin

package forticlient

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFillScriptCompiles(t *testing.T) {
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("osacompile not available")
	}
	for _, pass := range []string{"secret", `p"ass\word`, "รหัสลับ"} {
		script := buildFillScript("ming", pass, false)
		cmd := exec.Command("osacompile", "-o", filepath.Join(t.TempDir(), "fill.scpt"), "-")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fill AppleScript does not compile: %v\n%s", err, out)
		}
	}
}
