package update

import "testing"

func TestPickAssetURL(t *testing.T) {
	assets := []Asset{
		{Name: "Tobiro-1.0.0-darwin-universal.zip", BrowserDownloadURL: "https://example.com/mac.zip"},
		{Name: "Tobiro-1.0.0-windows-amd64.zip", BrowserDownloadURL: "https://example.com/win.zip"},
	}
	if got := PickAssetURL("darwin", "arm64", assets); got != "https://example.com/mac.zip" {
		t.Fatalf("darwin: got %q", got)
	}
	if got := PickAssetURL("windows", "amd64", assets); got != "https://example.com/win.zip" {
		t.Fatalf("windows: got %q", got)
	}
}
