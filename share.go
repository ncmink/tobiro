package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tobiro/internal/domain"
	"tobiro/internal/transfer"
)

const shareFileName = "biebie-connections.yaml"

func (a *App) ExportConnections() (ExportResultView, error) {
	all := a.profiles.List()
	if len(all) == 0 {
		return ExportResultView{}, fmt.Errorf("there is nothing to export yet — add a connection first")
	}
	profiles := filterProfilesForExport(all, a.profiles.GroupHidden)
	if len(profiles) == 0 {
		return ExportResultView{}, fmt.Errorf("there is nothing to export — every connection is in a hidden group")
	}

	raw, err := transfer.Marshal(transfer.Build(profiles, a.credentials.ListForProfile))
	if err != nil {
		return ExportResultView{}, err
	}

	dialog := application.Get().Dialog.SaveFile()
	dialog.SetMessage("Export connections")
	dialog.SetFilename(shareFileName)
	dialog.CanCreateDirectories(true)
	dialog.AddFilter("Tobiro connections", "*.yaml;*.yml")

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return ExportResultView{}, fmt.Errorf("could not open the save dialog: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return ExportResultView{Cancelled: true}, nil
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return ExportResultView{}, fmt.Errorf("could not write %s: %w", path, err)
	}

	a.log.Info("Exported %d connections to %s (no passwords included)", len(profiles), path)
	return ExportResultView{File: path, Connections: len(profiles)}, nil
}

func (a *App) ImportConnections() (ImportResultView, error) {
	dialog := application.Get().Dialog.OpenFile()
	dialog.SetMessage("Import connections")
	dialog.AddFilter("Tobiro connections", "*.yaml;*.yml")

	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return ImportResultView{}, fmt.Errorf("could not open the file dialog: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return ImportResultView{Cancelled: true}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return ImportResultView{}, fmt.Errorf("could not read %s: %w", path, err)
	}
	doc, err := transfer.Parse(raw)
	if err != nil {
		return ImportResultView{}, err
	}

	byName := make(map[string]string)
	for _, p := range a.profiles.List() {
		byName[strings.ToLower(strings.TrimSpace(p.Name))] = p.ID
	}
	fresh := make(map[string]string)

	result := ImportResultView{File: path}
	for _, conn := range doc.Connections {
		name := strings.TrimSpace(conn.Name)
		if _, exists := byName[strings.ToLower(name)]; exists {
			result.Skipped = append(result.Skipped, name)
			continue
		}

		created, err := a.profiles.Create(conn.ProfileInput())
		if err != nil {
			return result, fmt.Errorf("could not import %q: %w", name, err)
		}
		byName[strings.ToLower(name)] = created.ID
		fresh[strings.ToLower(name)] = created.ID
		result.Connections++

		for _, account := range conn.Accounts {
			if _, err := a.credentials.Create(created.ID, account.CredentialInput()); err != nil {
				return result, fmt.Errorf("imported %q, but its account %q failed: %w", name, account.Label, err)
			}
			result.Accounts++
		}
	}

	a.applyRequirements(doc.Connections, byName, fresh)

	if result.Connections > 0 {
		a.bridge.Notify(domain.EventProfileChanged, "")
	}
	a.log.Info("Imported %d connections and %d accounts from %s (%d skipped as already present)",
		result.Connections, result.Accounts, path, len(result.Skipped))
	return result, nil
}

func filterProfilesForExport(profiles []domain.VPNProfile, groupHidden func(string) bool) []domain.VPNProfile {
	if len(profiles) == 0 {
		return nil
	}
	byID := make(map[string]domain.VPNProfile, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}

	exportable := func(p domain.VPNProfile) bool {
		if groupHidden(p.Group) {
			return false
		}
		seen := make(map[string]struct{})
		parentID := strings.TrimSpace(p.Requires)
		for parentID != "" {
			if _, loop := seen[parentID]; loop {
				break
			}
			seen[parentID] = struct{}{}
			parent, ok := byID[parentID]
			if !ok {
				break
			}
			if groupHidden(parent.Group) {
				return false
			}
			parentID = strings.TrimSpace(parent.Requires)
		}
		return true
	}

	out := make([]domain.VPNProfile, 0, len(profiles))
	for _, p := range profiles {
		if exportable(p) {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) applyRequirements(connections []transfer.Connection, byName, fresh map[string]string) {
	for _, conn := range connections {
		needs := strings.TrimSpace(conn.Requires)
		if needs == "" {
			continue
		}
		id, imported := fresh[strings.ToLower(strings.TrimSpace(conn.Name))]
		if !imported {
			continue
		}
		target, found := byName[strings.ToLower(needs)]
		if !found {
			a.log.Warn("%q needs %q first, which is not in this file or on this machine — left unset",
				conn.Name, needs)
			continue
		}
		if _, err := a.profiles.SetRequirement(id, target); err != nil {
			a.log.Warn("could not record that %q needs %q first: %v", conn.Name, needs, err)
		}
	}
}
