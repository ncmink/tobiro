// Package sysexec runs vendor client executables safely.
//
// Every command is built as an argument array — there is no shell involved, so
// user-controlled values (host, port, profile name) cannot be interpreted as
// shell syntax.
package sysexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"tobiro/internal/domain"
)

// Result is the outcome of running a command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Combined returns stdout and stderr joined, for status parsing.
func (r Result) Combined() string {
	return strings.TrimSpace(r.Stdout + "\n" + r.Stderr)
}

// DefaultTimeout bounds a vendor CLI call so a hung client cannot wedge the UI.
const DefaultTimeout = 90 * time.Second

// Run executes an absolute executable path with an argument array.
//
// Note the deliberate absence of any "sh -c" path: constructing a shell command
// from user input is never permitted in this codebase.
func Run(ctx context.Context, executable string, args ...string) (Result, error) {
	return RunWithInput(ctx, executable, "", args...)
}

// RunWithInput is Run, additionally writing stdin — the safe way to hand a
// password to a client that reads one, since it never appears in the process
// argument list (which is world-readable via ps).
func RunWithInput(ctx context.Context, executable string, stdin string, args ...string) (Result, error) {
	if err := domain.ValidateExecutablePath(executable); err != nil {
		return Result{}, err
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\n\r\x00") {
			return Result{}, fmt.Errorf("argument contains illegal control characters")
		}
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: cmd.ProcessState.ExitCode(),
	}
	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("%s timed out after %s", executable, DefaultTimeout)
	}
	return result, err
}

// Exists reports whether a path is present on disk.
func Exists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// FirstExisting returns the first path that exists, or "".
func FirstExisting(paths ...string) string {
	for _, p := range paths {
		if Exists(p) {
			return p
		}
	}
	return ""
}

// LookPath resolves an executable on PATH, returning "" when absent.
func LookPath(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

// BundleVersion reads CFBundleShortVersionString from a macOS .app bundle.
//
// PlistBuddy is used because Info.plist may be stored in binary form, and it
// handles both encodings. Returns "" when the version cannot be read.
func BundleVersion(ctx context.Context, appPath string) string {
	const plistBuddy = "/usr/libexec/PlistBuddy"
	if !Exists(plistBuddy) || !Exists(appPath) {
		return ""
	}
	for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
		res, err := Run(ctx, plistBuddy, "-c", "Print :"+key, appPath+"/Contents/Info.plist")
		if err != nil {
			continue
		}
		if v := strings.TrimSpace(res.Stdout); v != "" && !strings.Contains(v, "Does Not Exist") {
			return v
		}
	}
	return ""
}

// OpenApplication launches a GUI application without blocking.
//
// The vendor GUI is the documented fallback whenever an adapter cannot drive
// the client programmatically.
func OpenApplication(ctx context.Context, path string) error {
	if !Exists(path) {
		return fmt.Errorf("%s: %w", path, domain.ErrClientNotInstalled)
	}

	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(path, ".app"):
		// macOS bundle.
		cmd = exec.CommandContext(ctx, "/usr/bin/open", path)
	default:
		if err := domain.ValidateExecutablePath(path); err != nil {
			return err
		}
		cmd = exec.CommandContext(ctx, path)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", path, err)
	}
	// Detach: the GUI outlives this call.
	go func() { _ = cmd.Wait() }()
	return nil
}
