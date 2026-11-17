//go:build darwin

package openvpn

import (
	"os"
	"path/filepath"
)

var darwinAppPaths = []string{
	"/Applications/OpenVPN Connect/OpenVPN Connect.app",
	"/Applications/OpenVPN Connect.app",
}

func platformAppPaths() []string { return darwinAppPaths }

func cliFromApp(appPath string) string {
	return filepath.Join(appPath, "Contents/MacOS/OpenVPN Connect")
}

func platformLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "OpenVPN Connect", "log", "ovpn.log")
}