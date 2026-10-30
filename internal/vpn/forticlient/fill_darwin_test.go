//go:build darwin

package forticlient

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFortiClientFillScriptCompiles(t *testing.T) {
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("osacompile not available")
	}
	script := buildFillScript("ming", `p"ass\word`, 8)
	for _, banned := range []string{"keystroke return", "key code 36"} {
		if strings.Contains(script, banned) {
			t.Fatalf("fill script must not contain %q", banned)
		}
	}
	if strings.Contains(script, `p"ass\word`) {
		t.Fatal("password must be encoded, not embedded raw")
	}
	cmd := exec.Command("osacompile", "-o", filepath.Join(t.TempDir(), "fill.scpt"), "-")
	cmd.Stdin = strings.NewReader(script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("FortiClient fill script does not compile: %v\n%s", err, out)
	}
}
