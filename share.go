package main

import (
	"fmt"
	"os"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tobiro/internal/domain"
	"tobiro/internal/transfer"
)

const shareFileName = "biebie-connections.yaml"

var shareFilters = []wailsruntime.FileFilter{
	{DisplayName: "Tobiro connections (*.yaml, *.yml)", Pattern: "*.yaml;*.yml"},
}

// ExportConnections writes every connection to a YAML file the user picks, so
// it can be handed to a colleague. Passwords are never part of the file — the
// recipient adds their own after importing.
func (a *App) ExportConnections() (ExportResultView, error) {
	profiles := a.profiles.List()
	if len(profiles) == 0 {
		return ExportResultView{}, fmt.Errorf("there is nothing to export yet — add a connection first")
	}

	raw, err := transfer.Marshal(transfer.Build(profiles, a.credentials.ListForProfile))
	if err != nil {
		return ExportResultView{}, err
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Export connections",
		DefaultFilename:      shareFileName,
		CanCreateDirectories: true,
		Filters:              shareFilters,
	})
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

// ImportConnections adds the connections from an exported YAML file.
//
// A connection whose name already exists is skipped rather than merged or
// duplicated, so re-importing a file a colleague updated is safe.
func (a *App) ImportConnections() (ImportResultView, error) {
	path, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Import connections",
		Filters: shareFilters,
	})
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

	// Every name on this machine, whether it arrived just now or was already
	// here, so a connection can be told which one it needs first.
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

// applyRequirements records "needs this up first" in a second pass, once every
// name in the file has an identifier on this machine.
//
// Only the connections this import created are touched: one that was skipped
// because it already existed keeps whatever order the user set up here. A
// reference to a name that is nowhere to be found leaves the connection
// standing alone, which is worth a log line but not a failed import.
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
