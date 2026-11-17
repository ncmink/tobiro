//go:build !darwin && !windows

package openvpn

func platformAppPaths() []string { return nil }

func cliFromApp(appPath string) string { return appPath }

func platformLogPath() string { return "" }