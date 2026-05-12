package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ReleaseAsset struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
}

type Release struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	PublishedAt string         `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Body        string         `json:"body"`
	Assets      []ReleaseAsset `json:"assets"`
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) FetchLatestRelease(ctx context.Context, repo string) (*Release, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return nil, errors.New("github repo is empty")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "v.wingsnet.org/1.0")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned %d", response.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func PickPrimaryAsset(release *Release, suffix string) *ReleaseAsset {
	if release == nil {
		return nil
	}
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	for index := range release.Assets {
		asset := &release.Assets[index]
		if suffix != "" && strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
			return asset
		}
	}
	for index := range release.Assets {
		asset := &release.Assets[index]
		if strings.HasSuffix(strings.ToLower(asset.Name), ".apk") {
			return asset
		}
	}
	if len(release.Assets) == 0 {
		return nil
	}
	return &release.Assets[0]
}
