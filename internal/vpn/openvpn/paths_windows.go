//go:build windows

package openvpn

import (
	"os"
	"path/filepath"
)

func platformAppPaths() []string { return nil }

func cliFromApp(appPath string) string { return appPath }

func platformLogPath() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		p := filepath.Join(local, "OpenVPN Connect", "log", "ovpn.log")
		if fileExists(p) {
			return p
		}
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		p := filepath.Join(appData, "OpenVPN Connect", "log", "ovpn.log")
		if fileExists(p) {
			return p
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "OpenVPN Connect", "log", "ovpn.log")
	}
	return ""
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}