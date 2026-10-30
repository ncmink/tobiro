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
		for _, probe := range []bool{false, true} {
			script := buildFillScript("ming", pass, probe, 4)
			cmd := exec.Command("osacompile", "-o", filepath.Join(t.TempDir(), "fill.scpt"), "-")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fill AppleScript (probe=%v) does not compile: %v\n%s", probe, err, out)
			}
		}
	}
}
