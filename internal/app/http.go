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
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/cmd/maintenant/web"
	mcpoauth "github.com/kolapsis/maintenant/internal/mcp/oauth"
	"github.com/kolapsis/maintenant/internal/store/sqlite"
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

// buildHTTPServer assembles the top-level HTTP mux and creates the server.
func (a *App) buildHTTPServer() *http.Server {
	topMux := http.NewServeMux()

	if a.cfg.MCP.Enabled {
		mcpHTTPHandler := gomcp.NewStreamableHTTPHandler(func(_ *http.Request) *gomcp.Server {
			return a.mcpServer
		}, nil)
		var mcpHandler http.Handler = mcpHTTPHandler

		if a.cfg.MCP.ClientID != "" && a.cfg.MCP.ClientSecret != "" {
			mcpOAuthStore := sqlite.NewMCPOAuthStore(a.db)
			oauthSrv := mcpoauth.NewOAuthServer(mcpoauth.Config{
				ClientID:     a.cfg.MCP.ClientID,
				ClientSecret: a.cfg.MCP.ClientSecret,
				IssuerURL:    a.cfg.BaseURL,
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
		} else {
			a.logger.Info("MCP server enabled without auth")
		}
		mcpHandler = a.rl.Middleware(mcpHandler)
		topMux.Handle("/mcp", mcpHandler)
		topMux.Handle("/mcp/", mcpHandler)
	}

	a.statusHandler.Register(topMux, a.rl.Middleware)
	topMux.Handle("/api/", a.router.Handler())
	topMux.Handle("/ping/", a.rl.Middleware(a.router.Handler()))
	topMux.Handle("/", SPAHandler(a.router.Handler(), a.logger))

	return &http.Server{
		Addr:         a.cfg.Addr,
		Handler:      WithRequestTimeout(topMux, 10*time.Second),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}
