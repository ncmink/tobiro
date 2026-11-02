package main

import (
	"context"
	"encoding/base64"
	_ "embed"
	"fmt"
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"

	"tobiro/internal/update"
)

const updateRepository = "ncmink/tobiro"

//go:embed updater.key.pub
var updaterPublicKey []byte

//go:embed build/appicon.png
var updaterLogoPNG []byte

const updaterWindowCSS = `:root {
	--bg: #14151a;
	--surface: #17171c;
	--surface-2: #1f1f26;
	--fg: #f4f4f5;
	--fg-dim: #a1a1aa;
	--fg-faint: #71717a;
	--border: #2a2a33;
	--accent: #7c9cff;
	--accent-fg: #14151a;
	--success: #22c55e;
	--error: #ef4444;
	--radius: 10px;
}

.u__btn--primary:hover:not(:disabled) {
	filter: none;
	background: #6a8ef0;
}

.u__btn--primary:active:not(:disabled) {
	filter: none;
	background: #5a7ee0;
}

.u--up-to-date .u__icon,
.u--ready .u__icon {
	color: transparent;
	font-size: 0;
	line-height: 0;
	background-color: var(--surface-2);
	background-image: url('%s');
	background-size: 34px 34px;
	background-position: center;
	background-repeat: no-repeat;
	border: 1px solid var(--border);
}`

func updaterWindowStyles() string {
	logo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(updaterLogoPNG)
	return fmt.Sprintf(updaterWindowCSS, logo)
}

func configureUpdater(app *application.App) {
	gh, err := github.New(github.Config{
		Repository:    updateRepository,
		ChecksumAsset: "SHA256SUMS",
		AssetMatcher:  update.MatchAsset,
		HTTPClient:    update.NewHTTPClient(),
	})
	if err != nil {
		log.Printf("update source could not be configured: %v", err)
		return
	}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: appVersion,
		PublicKey:      updaterPublicKey,
		Providers:      []updater.Provider{update.WithSignatures(gh)},
		Window: &updater.BuiltinWindow{
			CSS: updaterWindowStyles(),
		},
	}); err != nil {
		log.Printf("updater could not start: %v", err)
		return
	}

	var once sync.Once
	app.Event.On(updater.EventUpdateReady, func(*application.CustomEvent) {
		go once.Do(func() {
			if err := app.Updater.Restart(context.Background()); err != nil {
				log.Printf("update could not restart: %v", err)
			}
		})
	})
}
