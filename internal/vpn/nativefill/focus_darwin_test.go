//go:build darwin

package nativefill

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFocusScriptCompiles(t *testing.T) {
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("osacompile not available")
	}
	for _, pass := range []string{"secret", `p"ass\word`, "รหัสลับ"} {
		script := buildFocusFillScript([]string{"FortiClientAgent", "FortiClient"}, []string{"Forcepoint"}, "ming", pass)
		for _, want := range []string{"AXFocusedUIElement", "typeSecret", "findLoginFields", "listRunning"} {
			if !strings.Contains(script, want) {
				t.Fatalf("focus script missing %q", want)
			}
		}
		for _, banned := range []string{"keystroke return", "key code 36"} {
			if strings.Contains(script, banned) {
				t.Fatalf("focus script must not contain %q", banned)
			}
		}
		if strings.Contains(script, `p"ass\word`) {
			t.Fatal("password must be encoded, not embedded raw")
		}
		cmd := exec.Command("osacompile", "-o", filepath.Join(t.TempDir(), "focus.scpt"), "-")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("focus AppleScript does not compile: %v\n%s", err, out)
		}
	}
}
