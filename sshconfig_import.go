package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/sshconfig"
)

func sshConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "config"), nil
}

// ListSSHConfigHosts reads ~/.ssh/config and reports each Host block.
//
// A missing file is not an error; it returns an empty list. Each entry is
// marked when a connection with the same name already exists in Tobiro.
func (a *App) ListSSHConfigHosts() ([]SSHConfigHostView, error) {
	path, err := sshConfigPath()
	if err != nil {
		return nil, a.describe(err)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	byName := make(map[string]struct{})
	for _, p := range a.profiles.List() {
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = struct{}{}
	}

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	out := make([]SSHConfigHostView, 0, len(hosts))
	for _, h := range hosts {
		_, exists := byName[strings.ToLower(strings.TrimSpace(h.Alias))]
		out = append(out, SSHConfigHostView{
			Alias:        h.Alias,
			HostName:     h.HostName,
			User:         h.User,
			Port:         h.Port,
			ForwardCount: len(h.Forwards),
			Exists:       exists,
		})
	}
	return out, nil
}

// ImportSSHConfigHosts creates SSH tunnels from selected Host aliases in
// ~/.ssh/config. Each tunnel requires the given VPN profile and inherits its
// group.
func (a *App) ImportSSHConfigHosts(parentID string, aliases []string) (ImportResultView, error) {
	parent, err := a.profiles.Get(parentID)
	if err != nil {
		return ImportResultView{}, a.describe(err)
	}
	if parent.IsSSH() {
		return ImportResultView{}, fmt.Errorf("%q is an SSH tunnel — pick a VPN connection to import through", parent.Name)
	}

	path, err := sshConfigPath()
	if err != nil {
		return ImportResultView{}, a.describe(err)
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImportResultView{}, fmt.Errorf("there is no SSH config at %s", path)
		}
		return ImportResultView{}, fmt.Errorf("could not read %s: %w", path, err)
	}

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		return ImportResultView{}, fmt.Errorf("could not read %s: %w", path, err)
	}

	byAlias := make(map[string]sshconfig.Host, len(aliases))
	for _, h := range hosts {
		byAlias[strings.ToLower(strings.TrimSpace(h.Alias))] = h
	}

	byName := make(map[string]string)
	for _, p := range a.profiles.List() {
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = p.ID
	}

	result := ImportResultView{File: path}
	for _, alias := range aliases {
		key := strings.ToLower(strings.TrimSpace(alias))
		h, ok := byAlias[key]
		if !ok {
			continue
		}
		name := strings.TrimSpace(h.Alias)
		if name == "" {
			continue
		}
		if _, exists := byName[key]; exists {
			result.Skipped = append(result.Skipped, name)
			continue
		}

		created, err := a.profiles.Create(domain.ProfileInput{
			Name:     name,
			Kind:     domain.KindSSH,
			Provider: domain.ProviderSSH,
			Group:    parent.Group,
			Requires: parent.ID,
		})
		if err != nil {
			return result, fmt.Errorf("could not import %q: %w", name, err)
		}

		_, err = a.profiles.SetTunnel(created.ID, domain.TunnelInput{
			Host: h.HostName,
			Port: h.Port,
			SSH: domain.SSHSettings{
				User:     h.User,
				Forwards: append([]domain.PortForward(nil), h.Forwards...),
			},
		})
		if err != nil {
			return result, fmt.Errorf("could not configure %q: %w", name, err)
		}

		label := strings.TrimSpace(h.User)
		if label == "" {
			label = name
		}
		credIn := domain.CredentialInput{
			Label:     label,
			Username:  h.User,
			IsDefault: true,
		}
		if keyPath := strings.TrimSpace(h.IdentityFile); keyPath != "" {
			credIn.AuthMethod = domain.AuthSSHKey
			credIn.PrivateKeyPath = keyPath
		} else {
			credIn.AuthMethod = domain.AuthPassword
		}
		if _, err := a.credentials.Create(created.ID, credIn); err != nil {
			return result, fmt.Errorf("imported %q, but its account failed: %w", name, err)
		}

		byName[key] = created.ID
		result.Connections++
		result.Accounts++
	}

	if result.Connections > 0 {
		a.bridge.Notify(domain.EventProfileChanged, "")
	}
	a.log.Info("Imported %d SSH tunnels from %s through %q (%d skipped as already present)",
		result.Connections, path, parent.Name, len(result.Skipped))
	return result, nil
}
