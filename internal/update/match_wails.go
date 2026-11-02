// Package update chooses GitHub Release assets and authenticates updater downloads.
package update

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// MatchAsset picks the zip the Wails updater can swap into place.
func MatchAsset(req updater.CheckRequest, assets []github.ReleaseAsset) int {
	for _, suffix := range updaterZipSuffixes(req.Platform, req.Arch) {
		for i, asset := range assets {
			if strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
				return i
			}
		}
	}
	return -1
}

func updaterZipSuffixes(platform, arch string) []string {
	plat := strings.ToLower(platform)
	arch = strings.ToLower(arch)
	switch plat {
	case "darwin":
		return []string{"darwin-universal.zip", "darwin-" + arch + ".zip"}
	case "windows":
		return []string{"windows-" + arch + ".zip"}
	default:
		if plat == "" || arch == "" {
			return nil
		}
		return []string{plat + "-" + arch + ".zip"}
	}
}
