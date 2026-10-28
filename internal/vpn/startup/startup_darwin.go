//go:build darwin

package startup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
)

const (
	launchctl = "/bin/launchctl"
	defaults  = "/usr/bin/defaults"
	osascript = "/usr/bin/osascript"
)

// Read reports whether any of the vendor's login registrations are active.
func Read(ctx context.Context, spec Spec) (domain.StartupState, error) {
	enabled := false

	if spec.PrefDomain != "" && spec.PrefKey != "" {
		on, ok, err := readBoolPref(ctx, spec.PrefDomain, spec.PrefKey)
		if err != nil {
			return domain.StartupState{}, err
		}
		if ok && on {
			enabled = true
		}
	}

	disabled, err := readDisabledMap(ctx)
	if err != nil {
		return domain.StartupState{}, err
	}
	for _, label := range spec.Labels {
		state, known := disabled[label]
		if known && !state {
			enabled = true
		}
	}

	names, err := listLoginItems(ctx)
	if err != nil {
		return domain.StartupState{}, err
	}
	for _, name := range names {
		if matchesName(spec.LoginItemNames, name) {
			enabled = true
			break
		}
	}

	detail := "This client stays closed until you open it from Tobiro."
	if enabled {
		detail = "This client opens when you log in to the Mac."
	}
	return domain.StartupState{Known: true, Enabled: enabled, Detail: detail}, nil
}

// Set enables or disables the vendor GUI at login.
func Set(ctx context.Context, spec Spec, enabled bool) error {
	if spec.PrefDomain != "" && spec.PrefKey != "" {
		if err := writeBoolPref(ctx, spec.PrefDomain, spec.PrefKey, enabled); err != nil {
			return err
		}
	}

	action := "disable"
	if enabled {
		action = "enable"
	}
	uid := os.Getuid()
	for _, label := range spec.Labels {
		target := fmt.Sprintf("gui/%d/%s", uid, label)
		if _, err := sysexec.Run(ctx, launchctl, action, target); err != nil {
			return fmt.Errorf("%s %s: %w", action, label, err)
		}
	}

	if enabled {
		if spec.AppPath != "" && len(spec.LoginItemNames) > 0 {
			return addLoginItem(ctx, spec.LoginItemNames[0], spec.AppPath)
		}
		return nil
	}
	return removeLoginItems(ctx, spec.LoginItemNames)
}

func readDisabledMap(ctx context.Context) (map[string]bool, error) {
	target := fmt.Sprintf("gui/%d", os.Getuid())
	res, err := sysexec.Run(ctx, launchctl, "print-disabled", target)
	if err != nil {
		// An empty domain is not fatal — treat as "nothing recorded".
		if strings.TrimSpace(res.Combined()) == "" {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read login services: %s", defaultString(strings.TrimSpace(res.Combined()), err.Error()))
	}
	return parseDisabledMap(res.Stdout), nil
}

func readBoolPref(ctx context.Context, domainName, key string) (value bool, ok bool, err error) {
	if err := validatePref(domainName, key); err != nil {
		return false, false, err
	}
	res, err := sysexec.Run(ctx, defaults, "read", domainName, key)
	if err != nil {
		return false, false, nil
	}
	switch strings.ToLower(strings.TrimSpace(res.Stdout)) {
	case "1", "true", "yes":
		return true, true, nil
	case "0", "false", "no":
		return false, true, nil
	default:
		if n, convErr := strconv.Atoi(strings.TrimSpace(res.Stdout)); convErr == nil {
			return n != 0, true, nil
		}
		return false, false, nil
	}
}

func writeBoolPref(ctx context.Context, domainName, key string, enabled bool) error {
	if err := validatePref(domainName, key); err != nil {
		return err
	}
	flag := "false"
	if enabled {
		flag = "true"
	}
	_, err := sysexec.Run(ctx, defaults, "write", domainName, key, "-bool", flag)
	if err != nil {
		return fmt.Errorf("update %s: %w", key, err)
	}
	return nil
}

func validatePref(domainName, key string) error {
	if domainName == "" || key == "" {
		return fmt.Errorf("preference identifier is empty")
	}
	if strings.ContainsAny(domainName, " \t\n\"';|&$") || strings.ContainsAny(key, " \t\n\"';|&$") {
		return fmt.Errorf("preference identifier contains invalid characters")
	}
	return nil
}

func listLoginItems(ctx context.Context) ([]string, error) {
	script := `tell application "System Events" to get the name of every login item`
	res, err := sysexec.Run(ctx, osascript, "-e", script)
	if err != nil {
		combined := strings.TrimSpace(res.Combined())
		if strings.Contains(strings.ToLower(combined), "not authorized") ||
			strings.Contains(combined, "-1743") {
			return nil, fmt.Errorf("Tobiro needs Automation permission for System Events to change login items")
		}
		return nil, fmt.Errorf("read login items: %s", defaultString(combined, err.Error()))
	}
	raw := strings.TrimSpace(res.Stdout)
	if raw == "" {
		return nil, nil
	}
	var names []string
	for _, part := range strings.Split(raw, ",") {
		if n := strings.TrimSpace(part); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

func addLoginItem(ctx context.Context, name, appPath string) error {
	if err := domain.ValidateExecutablePath(appPath); err != nil {
		return err
	}
	if !sysexec.Exists(appPath) {
		return fmt.Errorf("application %s was not found", appPath)
	}
	script := fmt.Sprintf(
		`tell application "System Events" to if not (exists login item %s) then make login item at end with properties {path:%s, hidden:false, name:%s}`,
		quoteAS(name), quoteAS(appPath), quoteAS(name),
	)
	res, err := sysexec.Run(ctx, osascript, "-e", script)
	if err != nil {
		return fmt.Errorf("add login item: %s", defaultString(strings.TrimSpace(res.Combined()), err.Error()))
	}
	return nil
}

func removeLoginItems(ctx context.Context, names []string) error {
	if len(names) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("tell application \"System Events\"\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\tif exists login item %s then delete login item %s\n", quoteAS(name), quoteAS(name))
	}
	b.WriteString("end tell\n")
	res, err := sysexec.RunWithInput(ctx, osascript, b.String())
	if err != nil {
		return fmt.Errorf("remove login item: %s", defaultString(strings.TrimSpace(res.Combined()), err.Error()))
	}
	return nil
}

func quoteAS(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
