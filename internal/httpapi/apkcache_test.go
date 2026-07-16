package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"v.wingsnet.org/internal/githubapi"
)

// The cached asset must be served with an exact Content-Length: the browser
// progress bar has nothing to advance against without it, which is the whole
// reason the asset is cached instead of proxied.
func TestAPKCacheServesExactSizeAndRefreshesOnNewRelease(t *testing.T) {
	const v1 = "APK-BODY-VERSION-ONE"
	const v2 = "APK-BODY-VERSION-TWO-LONGER"

	body := v1
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Length", itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()

	c := newAPKCache(t.TempDir(), nil, "WINGS-N/WINGSV", ".apk")
	rel := &githubapi.Release{TagName: "v1.0.0"}
	asset := &githubapi.ReleaseAsset{Name: "WINGSV.apk", DownloadURL: upstream.URL}

	if err := c.ensure(context.Background(), rel, asset); err != nil {
		t.Fatalf("ensure v1: %v", err)
	}
	path, name, size, ok := c.cached("v1.0.0")
	if !ok {
		t.Fatal("v1 must be cached")
	}
	if size != int64(len(v1)) {
		t.Errorf("size = %d, want %d (an exact size is what drives the progress bar)", size, len(v1))
	}
	if name != "WINGSV.apk" {
		t.Errorf("filename = %q, want WINGSV.apk", name)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != v1 {
		t.Fatalf("cached bytes = %q (err %v), want %q", got, err, v1)
	}

	// A second ensure for the same tag must not re-download.
	if err := c.ensure(context.Background(), rel, asset); err != nil {
		t.Fatalf("ensure v1 again: %v", err)
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 (cached release must not re-download)", hits)
	}

	// A new release must replace it.
	body = v2
	rel2 := &githubapi.Release{TagName: "v2.0.0"}
	if err := c.ensure(context.Background(), rel2, asset); err != nil {
		t.Fatalf("ensure v2: %v", err)
	}
	if _, _, _, ok := c.cached("v1.0.0"); ok {
		t.Error("the old release must no longer be served once a new one is cached")
	}
	newPath, _, newSize, ok := c.cached("v2.0.0")
	if !ok {
		t.Fatal("v2 must be cached")
	}
	if newSize != int64(len(v2)) {
		t.Errorf("v2 size = %d, want %d", newSize, len(v2))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the superseded file must be removed, not left to pile up")
	}
	got, err = os.ReadFile(newPath)
	if err != nil || string(got) != v2 {
		t.Fatalf("v2 bytes = %q (err %v), want %q", got, err, v2)
	}
}

// A transfer that ends early but still returned 200 must not poison the cache:
// serving a truncated APK would be worse than not caching at all. (net/http
// usually catches the short body itself; the explicit size check backs that up.)
func TestAPKCacheRejectsTruncatedDownload(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer upstream.Close()

	c := newAPKCache(t.TempDir(), nil, "WINGS-N/WINGSV", ".apk")
	err := c.ensure(context.Background(),
		&githubapi.Release{TagName: "v1.0.0"},
		&githubapi.ReleaseAsset{Name: "WINGSV.apk", DownloadURL: upstream.URL})
	if err == nil {
		t.Fatal("ensure = nil, want an error for a body shorter than Content-Length")
	}
	if _, _, _, ok := c.cached("v1.0.0"); ok {
		t.Error("a truncated download must not be cached")
	}
	entries, _ := os.ReadDir(c.dir)
	if len(entries) != 0 {
		t.Errorf("cache dir holds %d file(s); the partial download must be cleaned up", len(entries))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
