package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/githubapi"
	"v.wingsnet.org/internal/guardianhub"
	adminhandler "v.wingsnet.org/internal/handlers/admin"
	guardianhandler "v.wingsnet.org/internal/handlers/guardian"
	ownerhandler "v.wingsnet.org/internal/handlers/owner"
	"v.wingsnet.org/internal/preview"
	"v.wingsnet.org/internal/storage"
	"v.wingsnet.org/web"
)

type Server struct {
	config        config.Config
	releaseClient *githubapi.Client
	staticHandler http.Handler
	store         *storage.Store
	hub           *guardianhub.Hub
	authSvc       *auth.Service
	adminH        *adminhandler.Handler
	guardianH     *guardianhandler.Handler
	ownerH        *ownerhandler.Handler
}

func New(cfg config.Config, store *storage.Store, authSvc *auth.Service, hub *guardianhub.Hub) *Server {
	return &Server{
		config:        cfg,
		releaseClient: githubapi.NewClient(),
		staticHandler: buildStaticHandler(cfg.StaticDir),
		store:         store,
		hub:           hub,
		authSvc:       authSvc,
		adminH:        adminhandler.New(cfg, store, authSvc, hub),
		guardianH:     guardianhandler.New(store, hub),
		ownerH:        ownerhandler.New(store, authSvc, hub),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/up", s.handleHealth)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/releases/latest", s.handleLatestRelease)
	mux.HandleFunc("/api/download/latest", s.handleLatestDownload)
	mux.HandleFunc("/.well-known/assetlinks.json", s.handleAssetLinks)
	s.adminH.Register(mux)
	s.adminH.RegisterWS(mux)
	s.ownerH.Register(mux)
	s.guardianH.Register(mux)
	mux.HandleFunc("/", s.handleFrontend)
	return redirectToHTTPS(s.config.PublicBaseURL, cors(s.config.PublicBaseURL, mux))
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handlePreview(writer http.ResponseWriter, request *http.Request) {
	rawLink := strings.TrimSpace(request.URL.Query().Get("link"))
	if rawLink == "" {
		writeError(writer, http.StatusBadRequest, "missing link")
		return
	}
	parsed, err := preview.Parse(rawLink)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, parsed)
}

func (s *Server) handleLatestRelease(writer http.ResponseWriter, request *http.Request) {
	release, asset, err := s.fetchLatestRelease(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"tagName":     release.TagName,
		"name":        release.Name,
		"publishedAt": release.PublishedAt,
		"htmlUrl":     release.HTMLURL,
		"body":        release.Body,
		"asset":       asset,
		"repo":        s.config.GitHubRepo,
	})
}

func (s *Server) handleLatestDownload(writer http.ResponseWriter, request *http.Request) {
	_, asset, err := s.fetchLatestRelease(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	if asset == nil || strings.TrimSpace(asset.DownloadURL) == "" {
		writeError(writer, http.StatusNotFound, "release asset not found")
		return
	}

	req, err := http.NewRequestWithContext(request.Context(), http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	req.Header.Set("User-Agent", "v.wingsnet.org/1.0")
	response, err := s.releaseClientHTTP().Do(req)
	if err != nil {
		writeError(writer, http.StatusBadGateway, err.Error())
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		writeError(writer, http.StatusBadGateway, "github download failed")
		return
	}

	filename := asset.Name
	if filename == "" {
		filename = "WINGSV.apk"
	}

	writer.Header().Set("Content-Type", firstNonEmpty(response.Header.Get("Content-Type"), "application/vnd.android.package-archive"))
	writer.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	writer.Header().Set("Cache-Control", "public, max-age=3600")
	if contentLength := response.Header.Get("Content-Length"); contentLength != "" {
		writer.Header().Set("Content-Length", contentLength)
	}
	// Disable proxy/HTTP-2 buffering so the browser sees bytes as they arrive
	// (otherwise Traefik holds the whole APK and progress jumps from 0% to 100%).
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flusher, _ := writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := response.Body.Read(buf)
		if n > 0 {
			if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (s *Server) handleAssetLinks(writer http.ResponseWriter, request *http.Request) {
	if strings.TrimSpace(s.config.AssetLinksJSON) != "" {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(s.config.AssetLinksJSON))
		return
	}
	if dir := strings.TrimSpace(s.config.StaticDir); dir != "" {
		staticPath := filepath.Join(dir, ".well-known", "assetlinks.json")
		if _, err := os.Stat(staticPath); err == nil {
			http.ServeFile(writer, request, staticPath)
			return
		}
	}
	if data, err := fs.ReadFile(web.Dist, "dist/.well-known/assetlinks.json"); err == nil {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(data)
		return
	}
	writeError(writer, http.StatusNotFound, "asset links not configured")
}

func (s *Server) handleFrontend(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/" || !strings.HasPrefix(request.URL.Path, "/api/") {
		s.staticHandler.ServeHTTP(writer, request)
		return
	}
	http.NotFound(writer, request)
}

func (s *Server) fetchLatestRelease(ctx context.Context) (*githubapi.Release, *githubapi.ReleaseAsset, error) {
	release, err := s.releaseClient.FetchLatestRelease(ctx, s.config.GitHubRepo)
	if err != nil {
		return nil, nil, err
	}
	asset := githubapi.PickPrimaryAsset(release, s.config.ReleaseAssetSuffix)
	if asset == nil {
		return nil, nil, errors.New("no release asset matched")
	}
	return release, asset, nil
}

func (s *Server) releaseClientHTTP() *http.Client {
	return &http.Client{Timeout: 0}
}

func buildStaticHandler(staticDir string) http.Handler {
	// Explicit STATIC_DIR keeps the legacy filesystem mode for ops who want to
	// swap the SPA without rebuilding the binary; with it unset we serve the
	// frontend embedded at compile time so the binary is self-contained.
	cleanDir := filepath.Clean(strings.TrimSpace(staticDir))
	if cleanDir != "" && cleanDir != "." {
		return buildDiskStaticHandler(cleanDir)
	}
	return buildEmbedStaticHandler()
}

func buildDiskStaticHandler(cleanDir string) http.Handler {
	indexPath := filepath.Join(cleanDir, "index.html")
	fileServer := http.FileServer(http.Dir(cleanDir))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath := filepath.Clean(strings.TrimPrefix(request.URL.Path, "/"))
		if requestPath == "." {
			http.ServeFile(writer, request, indexPath)
			return
		}
		if _, err := os.Stat(filepath.Join(cleanDir, requestPath)); err == nil {
			applyAssetCacheControl(writer, request.URL.Path)
			fileServer.ServeHTTP(writer, request)
			return
		}
		http.ServeFile(writer, request, indexPath)
	})
}

func buildEmbedStaticHandler() http.Handler {
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// At build time embed always succeeds; this branch only triggers if the
		// dist subdirectory itself is empty (developer forgot to run pnpm build).
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "frontend bundle missing", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		trimmed := strings.TrimPrefix(request.URL.Path, "/")
		if trimmed == "" {
			serveEmbedIndex(writer, request, distFS)
			return
		}
		cleaned := path.Clean(trimmed)
		if _, err := fs.Stat(distFS, cleaned); err == nil {
			applyAssetCacheControl(writer, request.URL.Path)
			fileServer.ServeHTTP(writer, request)
			return
		}
		serveEmbedIndex(writer, request, distFS)
	})
}

func serveEmbedIndex(writer http.ResponseWriter, request *http.Request, distFS fs.FS) {
	data, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		http.Error(writer, "frontend bundle missing", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(writer, request, "index.html", time.Time{}, strings.NewReader(string(data)))
}

func applyAssetCacheControl(w http.ResponseWriter, urlPath string) {
	switch {
	// Vite-hashed bundles never change at the same URL.
	case strings.HasPrefix(urlPath, "/assets/"):
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	// Fonts and the firstlaunch background art are large and stable.
	case strings.HasPrefix(urlPath, "/fonts/"),
		strings.HasPrefix(urlPath, "/img/"):
		w.Header().Set("Cache-Control", "public, max-age=2592000")
	}
}

func writeJSON(writer http.ResponseWriter, statusCode int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, statusCode int, message string) {
	writeJSON(writer, statusCode, map[string]any{
		"error":   true,
		"message": message,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cors(publicBaseURL string, next http.Handler) http.Handler {
	allowedOrigins := buildAllowedOrigins(publicBaseURL)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := strings.TrimSpace(request.Header.Get("Origin"))
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Vary", "Origin")
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func buildAllowedOrigins(publicBaseURL string) map[string]struct{} {
	allowed := map[string]struct{}{}
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base != "" {
		allowed[base] = struct{}{}
		switch {
		case strings.HasPrefix(base, "https://"):
			allowed["http://"+strings.TrimPrefix(base, "https://")] = struct{}{}
		case strings.HasPrefix(base, "http://"):
			allowed["https://"+strings.TrimPrefix(base, "http://")] = struct{}{}
		}
	}
	return allowed
}

func redirectToHTTPS(publicBaseURL string, next http.Handler) http.Handler {
	baseURL := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	baseHost := ""
	if parsed, err := url.Parse(baseURL); err == nil {
		baseHost = strings.ToLower(parsed.Host)
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.TLS != nil {
			next.ServeHTTP(writer, request)
			return
		}

		forwardedProto := strings.ToLower(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")))
		if forwardedProto == "https" {
			next.ServeHTTP(writer, request)
			return
		}

		if strings.HasPrefix(request.URL.Path, "/.well-known/acme-challenge/") {
			next.ServeHTTP(writer, request)
			return
		}

		// WebSocket upgrade requests must NOT be redirected — the 30x response
		// terminates the upgrade handshake on the client. Some proxies (and
		// HTTP/2-enabled edges) forward WS as HTTP/1.1 without setting
		// X-Forwarded-Proto, so we'd otherwise bounce them.
		if isWebSocketUpgrade(request) {
			next.ServeHTTP(writer, request)
			return
		}

		if baseURL == "" {
			next.ServeHTTP(writer, request)
			return
		}

		// Only "upgrade" requests targeting the canonical public host.
		// Loopback, lan IPs and any other host (staging, internal LB) get to
		// reach the app on plain HTTP — otherwise running the binary locally
		// against http://127.0.0.1:8080/ would 301 the user to v.wingsnet.org.
		requestHost := strings.ToLower(stripPort(request.Host))
		if requestHost == "" || isLoopbackOrPrivateHost(requestHost) {
			next.ServeHTTP(writer, request)
			return
		}
		if baseHost != "" && requestHost != stripPort(baseHost) {
			next.ServeHTTP(writer, request)
			return
		}

		targetURL := baseURL + request.URL.RequestURI()
		http.Redirect(writer, request, targetURL, http.StatusMovedPermanently)
	})
}

func stripPort(hostport string) string {
	if idx := strings.LastIndex(hostport, ":"); idx > 0 && !strings.Contains(hostport[idx:], "]") {
		return hostport[:idx]
	}
	return hostport
}

func isLoopbackOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, value := range r.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

func Run(ctx context.Context, cfg config.Config) error {
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.MarkAllClientsOffline(); err != nil {
		return err
	}
	authSvc := auth.New(store, cfg.SessionSecure)
	if err := authSvc.Bootstrap(cfg.BootstrapAdminUsername, cfg.BootstrapAdminPassword); err != nil {
		return err
	}
	if err := authSvc.EnsureAtLeastOneOwner(); err != nil {
		return err
	}
	hub := guardianhub.New()
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           New(cfg, store, authSvc, hub).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Audit log rotation: drop entries older than 30 days every 6h.
	const auditRetention = 30 * 24 * time.Hour
	go func() {
		_ = store.PruneAuditOlderThan(time.Now().Add(-auditRetention))
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = store.PruneAuditOlderThan(time.Now().Add(-auditRetention))
			}
		}
	}()

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	if listenErr := server.ListenAndServe(); !errors.Is(listenErr, http.ErrServerClosed) {
		return listenErr
	}
	<-shutdownDone
	return nil
}
