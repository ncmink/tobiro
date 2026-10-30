package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Repository is the GitHub repo whose Releases Tobiro checks (owner/name).
const Repository = "ncmink/tobiro"

// Release is the subset of a GitHub Release Tobiro needs.
type Release struct {
	TagName string
	HTMLURL string
	Assets  []Asset
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// FetchLatest loads the latest published GitHub Release for repo.
func FetchLatest(ctx context.Context, repo string, client *http.Client) (*Release, error) {
	if client == nil {
		client = NewHTTPClient()
	}
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, fmt.Errorf("release repository is not configured")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Tobiro-Update-Check")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact GitHub: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read GitHub response: %w", err)
	}
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no releases published yet for %s", repo)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", res.Status)
	}

	var raw githubRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse GitHub release: %w", err)
	}
	if strings.TrimSpace(raw.TagName) == "" {
		return nil, fmt.Errorf("GitHub release has no version tag")
	}

	out := &Release{
		TagName: strings.TrimSpace(raw.TagName),
		HTMLURL: strings.TrimSpace(raw.HTMLURL),
	}
	for _, a := range raw.Assets {
		if a.Name == "" || a.BrowserDownloadURL == "" {
			continue
		}
		out.Assets = append(out.Assets, Asset{
			Name:               a.Name,
			BrowserDownloadURL: a.BrowserDownloadURL,
		})
	}
	return out, nil
}
