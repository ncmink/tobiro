//go:build windows

package sysexec

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW from the Win32 process creation flags.
const createNoWindow = 0x08000000

// hideConsole stops Windows from opening a console window for a CLI child.
//
// Tobiro is a GUI-subsystem binary, so every console program it starts (the
// Tailscale CLI on each status poll, for one) would otherwise flash a new
// terminal window on screen.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
