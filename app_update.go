package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"

	"tobiro/internal/store"
	"tobiro/internal/update"
)

func (a *App) GetUpdateSettings() UpdateSettingsView {
	var auto bool
	a.store.Read(func(d *store.Data) {
		auto = d.AutoCheckUpdates
	})
	return UpdateSettingsView{AutoCheckUpdates: auto}
}

func (a *App) SetAutoCheckUpdates(enabled bool) error {
	return a.store.Update(func(d *store.Data) error {
		d.AutoCheckUpdates = enabled
		return nil
	})
}

// FrontendReady is called once the UI is listening. It may run a background
// release check when due.
func (a *App) FrontendReady() {
	var auto bool
	a.store.Read(func(d *store.Data) {
		auto = d.AutoCheckUpdates
	})
	if !auto {
		return
	}
	if !a.dueForAutoUpdateCheck() {
		return
	}
	go a.runUpdateCheck(false)
}

// CheckForUpdate opens the Wails update window and stages a newer release when
// one exists. The call returns once the check has started.
func (a *App) CheckForUpdate() error {
	if a.updater == nil || a.updater.State() == updater.StateUnconfigured {
		return errors.New("checking for updates is not available in this build")
	}
	go a.runUpdateCheck(true)
	return nil
}

func (a *App) dueForAutoUpdateCheck() bool {
	var lastCheck string
	a.store.Read(func(d *store.Data) {
		lastCheck = d.LastUpdateCheckAt
	})
	return update.DueForAutoCheck(lastCheck, time.Now())
}

func (a *App) recordUpdateCheck(now time.Time) {
	if err := a.store.Update(func(d *store.Data) error {
		d.LastUpdateCheckAt = now.UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		a.log.Warn("Could not record update check time: %v", err)
	}
}

func (a *App) runUpdateCheck(interactive bool) {
	if a.updater == nil || a.updater.State() == updater.StateUnconfigured {
		return
	}

	a.recordUpdateCheck(time.Now())

	if interactive {
		if err := a.updater.CheckAndInstall(context.Background()); err != nil {
			log.Printf("update: %v", err)
		}
		return
	}

	rel, err := a.updater.Check(context.Background())
	if err != nil || rel == nil {
		return
	}
	if err := a.updater.CheckAndInstall(context.Background()); err != nil {
		log.Printf("update: %v", err)
	}
}
