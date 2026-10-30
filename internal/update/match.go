package update

import "strings"

// Asset names a file attached to a GitHub Release.
type Asset struct {
	Name               string
	BrowserDownloadURL string
}

// PickAssetURL chooses a platform zip from release assets, if any.
func PickAssetURL(platform, arch string, assets []Asset) string {
	for _, suffix := range assetSuffixes(platform, arch) {
		for _, asset := range assets {
			if strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
				return asset.BrowserDownloadURL
			}
		}
	}
	return ""
}

func assetSuffixes(platform, arch string) []string {
	plat := strings.ToLower(platform)
	arch = strings.ToLower(arch)
	switch plat {
	case "darwin":
		return []string{"darwin-universal.zip", "darwin-" + arch + ".zip", "darwin-universal.dmg", "darwin-" + arch + ".dmg"}
	case "windows":
		return []string{"windows-" + arch + ".zip", "windows-" + arch + "-installer.exe", "windows-amd64-installer.exe"}
	default:
		if plat == "" || arch == "" {
			return nil
		}
		return []string{plat + "-" + arch + ".zip"}
	}
}
