//go:build !windows

package sysexec

import "os/exec"

func hideConsole(*exec.Cmd) {}
