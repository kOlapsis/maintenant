// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/cmd/maintenant/web"
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

// buildHTTPServer assembles the top-level HTTP mux and creates the server.
func (a *App) buildHTTPServer() *http.Server {
	topMux := http.NewServeMux()

	if a.cfg.MCP.Enabled {
		mcpHTTPHandler := gomcp.NewStreamableHTTPHandler(func(_ *http.Request) *gomcp.Server {
			return a.mcpServer
		}, nil)
		var mcpHandler http.Handler = mcpHTTPHandler

		if a.cfg.MCP.ClientID != "" && a.cfg.MCP.ClientSecret != "" {
			mcpOAuthStore := store.NewMCPOAuthStore(a.db)
			oauthSrv := mcpoauth.NewOAuthServer(mcpoauth.Config{
				ClientID:            a.cfg.MCP.ClientID,
				ClientSecret:        a.cfg.MCP.ClientSecret,
				IssuerURL:           a.cfg.BaseURL,
				AllowedRedirectURIs: a.cfg.MCP.AllowedRedirectURIs,
			}, mcpOAuthStore, a.logger.With("component", "mcp-oauth"))

			topMux.HandleFunc("/.well-known/oauth-authorization-server", oauthSrv.HandleAuthServerMetadata)
			topMux.HandleFunc("/oauth/authorize", oauthSrv.HandleAuthorize)
			topMux.HandleFunc("/oauth/token", oauthSrv.HandleToken)

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
			mcpHandler = authMiddleware(mcpHTTPHandler)

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
	topMux.Handle("/api/", a.apiRL.Middleware(a.router.Handler()))
	topMux.Handle("/ping/", a.rl.Middleware(a.router.Handler()))
	topMux.Handle("/", SPAHandler(a.router.Handler(), a.logger))

	handler := SecurityHeaders(contentSecurityPolicy(indexHTML))(
		WithRequestTimeout(topMux, 10*time.Second))

	return &http.Server{
		Addr:         a.cfg.Addr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}
