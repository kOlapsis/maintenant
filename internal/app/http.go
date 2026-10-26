// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kolapsis/maintenant/cmd/maintenant/web"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	mcpoauth "github.com/kolapsis/maintenant/internal/mcp/oauth"
	"github.com/kolapsis/maintenant/internal/store"
	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// SPAHandler returns an http.Handler that serves the embedded SPA frontend.
// API and ping routes are delegated to the API handler; everything else is
// served from the embedded filesystem, with a fallback to index.html for
// client-side routing.
func SPAHandler(apiHandler http.Handler, logger *slog.Logger) http.Handler {
	distFS, err := fs.Sub(web.FS, "dist")
	if err != nil {
		logger.Warn("SPA assets not embedded, frontend will not be served")
		return apiHandler
	}

	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ping/") {
			apiHandler.ServeHTTP(w, r)
			return
		}

		f, err := fs.Stat(distFS, strings.TrimPrefix(path, "/"))
		if err == nil && !f.IsDir() {
			if strings.HasPrefix(path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if path == "/sw.js" || path == "/registerSW.js" || path == "/manifest.webmanifest" {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback — serve index.html without caching
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// noStoreAPI marks every /api/v1/ answer as never to be stored; an SSE handler overrides it with no-cache.
func noStoreAPI(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") && w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", "no-store")
		}
		h.ServeHTTP(w, r)
	})
}

// IsStreamingPath reports whether path corresponds to an SSE or streaming endpoint.
func IsStreamingPath(path string) bool {
	if path == "/api/v1/containers/events" || path == "/status/events" {
		return true
	}
	if path == "/mcp" || strings.HasPrefix(path, "/mcp/") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/containers/") && strings.HasSuffix(path, "/logs/stream") {
		return true
	}
	return false
}

// WithRequestTimeout wraps non-streaming handlers with http.TimeoutHandler so
// that ordinary REST requests are bounded even though the server-level
// WriteTimeout is disabled (required for SSE).
func WithRequestTimeout(h http.Handler, timeout time.Duration) http.Handler {
	wrapped := http.TimeoutHandler(h, timeout, "request timeout")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsStreamingPath(r.URL.Path) {
			h.ServeHTTP(w, r)
			return
		}
		wrapped.ServeHTTP(w, r)
	})
}

// inlineScriptRe matches a <script> element and captures its attributes and its
// body. One carrying a src attribute is fetched from the origin and is already
// covered by 'self'; only an inline body needs a hash.
var inlineScriptRe = regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)

// inlineScriptHashes returns the CSP source expression for every inline script
// in html. The digest covers the element's text content byte for byte, which is
// what a browser hashes when matching the policy.
func inlineScriptHashes(html []byte) []string {
	var hashes []string
	for _, m := range inlineScriptRe.FindAllSubmatch(html, -1) {
		if bytes.Contains(bytes.ToLower(m[1]), []byte("src=")) {
			continue
		}
		if len(bytes.TrimSpace(m[2])) == 0 {
			continue
		}
		sum := sha256.Sum256(m[2])
		hashes = append(hashes, "sha256-"+base64.StdEncoding.EncodeToString(sum[:]))
	}
	return hashes
}

// contentSecurityPolicy builds the policy served with every response, minus the
// frame-ancestors directive that SecurityHeaders appends per route.
//
// It is computed at startup from the embedded index.html because script-src has
// to carry the sha256 of its inline bootstrap script (it applies the theme and
// density before the bundle loads, so the page does not flash). Hashing what is
// actually embedded keeps the policy correct across frontend rebuilds, where a
// hardcoded digest would rot silently and blank the app.
//
// style-src keeps 'unsafe-inline' because Vue and uPlot both set element styles
// at runtime. img-src allows data: for the status page logo and hero, which are
// stored and served as data URLs.
func contentSecurityPolicy(indexHTML []byte) string {
	scriptSrc := "'self'"
	for _, h := range inlineScriptHashes(indexHTML) {
		scriptSrc += " '" + h + "'"
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src " + scriptSrc,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"worker-src 'self'",
		"manifest-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")
}

// isPublicStatusPath reports whether path belongs to the public status page.
// The admin side of the status feature lives under /api/v1/status/ and is
// deliberately not matched here.
func isPublicStatusPath(path string) bool {
	return path == "/status" || strings.HasPrefix(path, "/status/")
}

// SecurityHeaders bounds what a browser will do with a response from this
// origin. Applied outside the timeout handler so the headers are on the wire
// even when it writes its own 503, and with Set so a route with a stricter
// policy of its own — the status page asset route — still wins by overwriting.
//
// Framing is refused everywhere except the public status page, which docs and
// SECURITY.md both describe as embeddable. The dashboard and the admin API are
// the surfaces a clickjacker would want, and they are the ones locked down.
//
// No HSTS here: TLS terminates at the reverse proxy, and deriving "this was
// HTTPS" from a forwarded header would trust something a client can forge.
// It belongs in the proxy config, where SECURITY.md puts it.
func SecurityHeaders(csp string) func(http.Handler) http.Handler {
	noFraming := csp + "; frame-ancestors 'none'"
	embeddable := csp + "; frame-ancestors *"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			if isPublicStatusPath(r.URL.Path) {
				h.Set("Content-Security-Policy", embeddable)
			} else {
				h.Set("X-Frame-Options", "DENY")
				h.Set("Content-Security-Policy", noFraming)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// hostname returns the lowercase host name of a Host header, without port or IPv6 brackets.
func hostname(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

// publicPath reports whether p is the public status page, a ping or a file the status page needs.
func publicPath(p string) bool {
	switch p {
	case "/status", "/manifest.webmanifest", "/sw.js", "/registerSW.js", "/apple-touch-icon.png", "/logo.svg":
		return true
	}
	for _, prefix := range []string{"/status/", "/assets/", "/ping/", "/screenshots/", "/workbox-", "/favicon", "/icon"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// statusHostFilter keeps a dedicated status host to the public paths and serves the status page at its root.
func statusHostFilter(statusHost, baseHost string, next http.Handler) http.Handler {
	if statusHost == "" || statusHost == baseHost {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hostname(r.Host) != statusHost {
			next.ServeHTTP(w, r)
			return
		}
		switch {
		case r.URL.Path == "/":
			r.URL.Path = "/status/"
			next.ServeHTTP(w, r)
		case publicPath(r.URL.Path):
			next.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

// hostGuard answers 421 when the Host of a request outside the public routes is neither an IP address nor an accepted name.
func hostGuard(allowed map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if publicPath(p) || p == "/mcp" || strings.HasPrefix(p, "/mcp/") || strings.HasPrefix(p, "/oauth/") || strings.HasPrefix(p, "/.well-known/") {
			next.ServeHTTP(w, r)
			return
		}
		name := hostname(r.Host)
		if _, ok := allowed[name]; !ok && net.ParseIP(name) == nil {
			v1.WriteError(w, http.StatusMisdirectedRequest, "misdirected_request", "Host not allowed: add it to MAINTENANT_BASE_URL or MAINTENANT_ALLOWED_HOSTS")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// forwardedHeadersWarning logs once that forwarded headers are ignored because no trusted proxy is declared.
func forwardedHeadersWarning(logger *slog.Logger, trusted bool, next http.Handler) http.Handler {
	if trusted {
		return next
	}
	var once sync.Once
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
			if r.Header.Get(header) != "" {
				once.Do(func() {
					logger.Warn("forwarded headers received but MAINTENANT_TRUSTED_PROXIES is empty: every visitor counts as the proxy", "header", header)
				})
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

const minMCPClientSecretLength = 32

// warnWeakMCPClientSecret warns when the only credential the MCP OAuth flow checks is short enough to guess.
func (a *App) warnWeakMCPClientSecret() {
	n := utf8.RuneCountInString(a.cfg.MCP.ClientSecret)
	if n >= minMCPClientSecretLength {
		return
	}
	a.logger.Warn("MAINTENANT_MCP_CLIENT_SECRET is shorter than 32 characters: /oauth/authorize approves every request, so this secret alone keeps /mcp closed",
		"length", n, "minimum", minMCPClientSecretLength, "fix", "generate one with: openssl rand -hex 32")
}

// unbufferedStream asks a buffering reverse proxy such as nginx to relay h's responses as they are written.
func unbufferedStream(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Accel-Buffering", "no")
		h.ServeHTTP(w, r)
	})
}

// buildHTTPServer assembles the top-level HTTP mux and creates the server.
func (a *App) buildHTTPServer() *http.Server {
	topMux := http.NewServeMux()

	if a.cfg.MCP.Enabled && !a.cfg.DemoMode {
		oauthConfigured := a.cfg.MCP.ClientID != "" && a.cfg.MCP.ClientSecret != ""
		mcpHandler := unbufferedStream(gomcp.NewStreamableHTTPHandler(func(_ *http.Request) *gomcp.Server {
			return a.mcpServer
		}, &gomcp.StreamableHTTPOptions{
			// A reverse proxy on the same host forwards the public Host, which the SDK refuses on a loopback listener.
			DisableLocalhostProtection: oauthConfigured,
		}))

		if oauthConfigured {
			a.warnWeakMCPClientSecret()
			mcpOAuthStore := store.NewMCPOAuthStore(a.db)
			oauthSrv := mcpoauth.NewOAuthServer(mcpoauth.Config{
				ClientID:            a.cfg.MCP.ClientID,
				ClientSecret:        a.cfg.MCP.ClientSecret,
				IssuerURL:           a.cfg.BaseURL,
				AllowedRedirectURIs: a.cfg.MCP.AllowedRedirectURIs,
			}, mcpOAuthStore, a.logger.With("component", "mcp-oauth"))

			topMux.HandleFunc("/.well-known/oauth-authorization-server", oauthSrv.HandleAuthServerMetadata)
			topMux.Handle("/oauth/authorize", a.rl.Middleware(http.HandlerFunc(oauthSrv.HandleAuthorize)))
			topMux.Handle("/oauth/token", a.rl.Middleware(http.HandlerFunc(oauthSrv.HandleToken)))

			topMux.Handle("/.well-known/oauth-protected-resource",
				mcpauth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
					Resource:               a.cfg.BaseURL + "/mcp",
					AuthorizationServers:   []string{a.cfg.BaseURL},
					BearerMethodsSupported: []string{"header"},
					ResourceName:           "maintenant MCP",
				}))

			resourceMetadataURL := a.cfg.BaseURL + "/.well-known/oauth-protected-resource"
			tokenVerifier := mcpoauth.NewTokenVerifier(mcpOAuthStore)
			authMiddleware := mcpauth.RequireBearerToken(tokenVerifier, &mcpauth.RequireBearerTokenOptions{
				ResourceMetadataURL: resourceMetadataURL,
			})
			mcpHandler = authMiddleware(mcpHandler)

			go mcpoauth.StartCleanup(context.Background(), mcpOAuthStore, a.logger.With("component", "mcp-oauth-cleanup"))

			a.logger.Info("MCP server enabled with OAuth2 auth", "client_id", a.cfg.MCP.ClientID)
		} else if a.cfg.MCP.AllowUnauthenticated {
			// The only way this route ever serves traffic: ValidateHTTP refuses
			// to listen without the opt-out. Warn, because it is a real exposure.
			a.logger.Warn("MCP server enabled WITHOUT auth — /mcp answers anyone who reaches it",
				"opt_out", "MAINTENANT_MCP_ALLOW_UNAUTHENTICATED")
		}
		mcpHandler = a.rl.Middleware(mcpHandler)
		topMux.Handle("/mcp", mcpHandler)
		topMux.Handle("/mcp/", mcpHandler)
	}

	// Pass the SPA index.html to the status handler so it can serve it for
	// /status/. The same bytes seed the CSP — its inline script needs a hash.
	var indexHTML []byte
	if distFS, err := fs.Sub(web.FS, "dist"); err == nil {
		if data, err := fs.ReadFile(distFS, "index.html"); err == nil {
			indexHTML = data
			a.statusHandler.SetIndexHTML(data)
		}
	}

	a.statusHandler.Register(topMux, a.rl.Middleware)
	topMux.Handle("/api/", noStoreAPI(a.apiRL.Middleware(a.router.Handler())))
	topMux.Handle("/ping/", a.router.Handler())
	topMux.Handle("/", SPAHandler(a.router.Handler(), a.logger))

	proxies, err := a.cfg.ParseTrustedProxies()
	trusted := err == nil && len(proxies) > 0
	// Start refuses an unreadable MAINTENANT_STATUS_URL through ValidateHTTP.
	statusHost, _ := a.cfg.StatusHost()
	handler := SecurityHeaders(contentSecurityPolicy(indexHTML))(
		WithRequestTimeout(v1.DemoModeGuard(a.cfg.DemoMode, a.cfg.DemoToken)(
			statusHostFilter(statusHost, urlHostname(a.cfg.BaseURL), hostGuard(a.cfg.AllowedHostSet(), forwardedHeadersWarning(a.logger, trusted, topMux)))), 10*time.Second))

	return &http.Server{
		Addr:         a.cfg.Addr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}
