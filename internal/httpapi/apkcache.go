package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"v.wingsnet.org/internal/githubapi"
)

// apkCache keeps the current release's asset on local disk.
//
// Without it every download proxied the bytes from GitHub per client: the size
// was only as trustworthy as GitHub's response, each visitor cost a full
// upstream transfer, and every page load spent a GitHub API call on release
// metadata (rate-limited). Serving a local file instead lets http.ServeContent
// set an exact Content-Length and honour Range requests, which is what makes the
// browser's progress bar move.
//
// The cache is per-pod and lives in ephemeral storage: losing it only costs one
// re-download. Metadata and the file refresh on their own schedule, so a new
// release is picked up without a restart.
type apkCache struct {
	dir          string
	client       *githubapi.Client
	httpClient   *http.Client
	repo         string
	assetSuffix  string
	metaTTL      time.Duration
	refreshEvery time.Duration

	mu       sync.RWMutex
	release  *githubapi.Release
	asset    *githubapi.ReleaseAsset
	fetched  time.Time
	tag      string // release tag the cached file belongs to
	path     string
	size     int64
	filename string

	// download is held for the whole fetch so N concurrent first-hits pull the
	// asset once instead of N times.
	download sync.Mutex
}

func newAPKCache(dir string, client *githubapi.Client, repo, assetSuffix string) *apkCache {
	return &apkCache{
		dir:          dir,
		client:       client,
		httpClient:   &http.Client{Timeout: 10 * time.Minute},
		repo:         repo,
		assetSuffix:  assetSuffix,
		metaTTL:      5 * time.Minute,
		refreshEvery: 15 * time.Minute,
	}
}

// metadata returns the latest release, refreshing from GitHub only once per
// metaTTL so page loads do not each spend an API call.
func (c *apkCache) metadata(ctx context.Context) (*githubapi.Release, *githubapi.ReleaseAsset, error) {
	c.mu.RLock()
	rel, asset, fetched := c.release, c.asset, c.fetched
	c.mu.RUnlock()
	if rel != nil && asset != nil && time.Since(fetched) < c.metaTTL {
		return rel, asset, nil
	}

	rel, err := c.client.FetchLatestRelease(ctx, c.repo)
	if err != nil {
		// Serve a stale copy rather than failing the page when GitHub is down or
		// rate-limiting us.
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.release != nil && c.asset != nil {
			return c.release, c.asset, nil
		}
		return nil, nil, err
	}
	asset = githubapi.PickPrimaryAsset(rel, c.assetSuffix)
	if asset == nil {
		return nil, nil, errors.New("no release asset matched")
	}

	c.mu.Lock()
	c.release, c.asset, c.fetched = rel, asset, time.Now()
	c.mu.Unlock()
	return rel, asset, nil
}

// cached returns the on-disk asset for tag, if this exact release is cached.
func (c *apkCache) cached(tag string) (path, filename string, size int64, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tag != tag || c.path == "" {
		return "", "", 0, false
	}
	return c.path, c.filename, c.size, true
}

// ensure downloads the asset for the current release unless it is already
// cached. Safe to call concurrently: the download lock collapses parallel calls.
func (c *apkCache) ensure(ctx context.Context, release *githubapi.Release, asset *githubapi.ReleaseAsset) error {
	tag := release.TagName
	if _, _, _, ok := c.cached(tag); ok {
		return nil
	}

	c.download.Lock()
	defer c.download.Unlock()
	// Another caller may have finished while we waited for the lock.
	if _, _, _, ok := c.cached(tag); ok {
		return nil
	}

	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	name := asset.Name
	if strings.TrimSpace(name) == "" {
		name = "WINGSV.apk"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "v.wingsnet.org/1.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github download failed: %s", resp.Status)
	}

	// Write to a temp file and rename: a half-written APK must never be served.
	tmp, err := os.CreateTemp(c.dir, "apk-*.part")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	size, err := io.Copy(tmp, resp.Body)
	closeErr := tmp.Close()
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}
	// A truncated transfer that still returned 200 would poison the cache.
	if resp.ContentLength > 0 && size != resp.ContentLength {
		_ = os.Remove(tmpName)
		return fmt.Errorf("short read: got %d of %d bytes", size, resp.ContentLength)
	}

	final := filepath.Join(c.dir, sanitizeCacheName(tag)+"-"+sanitizeCacheName(name))
	if err := os.Rename(tmpName, final); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	c.mu.Lock()
	old := c.path
	c.tag, c.path, c.size, c.filename = tag, final, size, name
	c.mu.Unlock()

	if old != "" && old != final {
		_ = os.Remove(old)
	}
	log.Printf("apk cache: stored %s (%s, %d bytes)", name, tag, size)
	return nil
}

// run refreshes the cached asset in the background so a new release is already
// on disk by the time someone clicks download.
func (c *apkCache) run(ctx context.Context) {
	ticker := time.NewTicker(c.refreshEvery)
	defer ticker.Stop()
	for {
		release, asset, err := c.metadata(ctx)
		if err == nil {
			if err := c.ensure(ctx, release, asset); err != nil {
				log.Printf("apk cache: refresh failed: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// metadata() re-fetches once its TTL lapses, so a new tag is noticed here.
			c.mu.Lock()
			c.fetched = time.Time{}
			c.mu.Unlock()
		}
	}
}

// sanitizeCacheName keeps a tag/filename usable as a path segment.
func sanitizeCacheName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}
