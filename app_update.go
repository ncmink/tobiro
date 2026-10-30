package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"tobiro/internal/domain"
	"tobiro/internal/store"
	"tobiro/internal/update"
)

const updateCheckTimeout = 45 * time.Second

// GetUpdateSettings reports whether Tobiro checks GitHub Releases at startup.
func (a *App) GetUpdateSettings() UpdateSettingsView {
	var auto bool
	a.store.Read(func(d *store.Data) {
		auto = d.AutoCheckUpdates
	})
	return UpdateSettingsView{AutoCheckUpdates: auto}
}

// SetAutoCheckUpdates enables or disables automatic release checks.
func (a *App) SetAutoCheckUpdates(enabled bool) error {
	return a.store.Update(func(d *store.Data) error {
		d.AutoCheckUpdates = enabled
		return nil
	})
}

// FrontendReady is called once the UI is listening for events. It may run a
// background release check when due.
func (a *App) FrontendReady() {
	a.maybeAutoCheckForUpdate()
}

// CheckForUpdate contacts GitHub and reports whether a newer release exists.
func (a *App) CheckForUpdate() (UpdateCheckView, error) {
	return a.runUpdateCheck(true)
}

// OpenUpdateURL opens a release or download link in the system browser.
func (a *App) OpenUpdateURL(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("no update link to open")
	}
	if a.ctx == nil {
		return fmt.Errorf("application is not ready")
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

func (a *App) maybeAutoCheckForUpdate() {
	var auto bool
	var lastCheck string
	a.store.Read(func(d *store.Data) {
		auto = d.AutoCheckUpdates
		lastCheck = d.LastUpdateCheckAt
	})
	if !auto {
		return
	}
	if !update.DueForAutoCheck(lastCheck, time.Now()) {
		return
	}
	go func() {
		view, err := a.runUpdateCheck(false)
		if err != nil {
			a.log.Warn("Update check: %v", err)
			return
		}
		if !view.UpdateAvailable {
			return
		}
		a.notifyUpdateAvailable(view)
	}()
}

func (a *App) notifyUpdateAvailable(view UpdateCheckView) {
	if a.bridge == nil {
		return
	}
	a.bridge.present()
	a.bridge.Notify(domain.EventUpdateAvailable, view)
}

func (a *App) recordUpdateCheck(now time.Time) {
	if err := a.store.Update(func(d *store.Data) error {
		d.LastUpdateCheckAt = now.UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		a.log.Warn("Could not record update check time: %v", err)
	}
}

func (a *App) runUpdateCheck(interactive bool) (UpdateCheckView, error) {
	a.recordUpdateCheck(time.Now())

	view := UpdateCheckView{Current: appVersion}
	ctx, cancel := context.WithTimeout(a.ctx, updateCheckTimeout)
	defer cancel()

	release, err := update.FetchLatest(ctx, update.Repository, update.NewHTTPClient())
	if err != nil {
		view.Message = err.Error()
		if interactive {
			return view, a.describe(err)
		}
		return view, err
	}

	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	view.Latest = latest
	view.ReleasePage = release.HTMLURL
	view.DownloadURL = update.PickAssetURL(runtime.GOOS, runtime.GOARCH, release.Assets)
	if view.DownloadURL == "" {
		view.DownloadURL = release.HTMLURL
	}

	if update.CompareVersions(appVersion, latest) >= 0 {
		view.Message = "You're on the latest version."
		return view, nil
	}

	view.UpdateAvailable = true
	view.Message = fmt.Sprintf("Tobiro %s is available.", release.TagName)
	return view, nil
}
