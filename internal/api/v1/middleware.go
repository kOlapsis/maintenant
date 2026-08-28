package v1

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kolapsis/maintenant/internal/extension"
)

type ctxKey int

const ctxKeyRequestID ctxKey = iota

// RequestIDFrom extracts the request ID from the context, if present.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// requestID assigns a short unique ID to each request and sets it on the
// context and response header.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()[:8]
		}
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusWriter captures the HTTP status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying ResponseWriter if it supports http.Flusher.
// This is required for SSE streaming to work through the middleware chain.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter so that http.ResponseController
// and interface assertions (e.g. http.Flusher) work correctly.
func (sw *statusWriter) Unwrap() http.ResponseWriter {
	return sw.ResponseWriter
}

// requestLogger logs each completed HTTP request at Debug level.
func requestLogger(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		logger.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestIDFrom(r.Context()),
		)
	})
}

// panicRecovery catches panics in downstream handlers and returns a 500.
func panicRecovery(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", RequestIDFrom(r.Context()),
				)
				WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// parseCORSOrigins parses a comma-separated CORS origins string.
// Returns nil for empty input (same-origin only).
func parseCORSOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	if raw == "*" {
		return []string{"*"}
	}
	var origins []string
	for _, p := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

// cors adds CORS headers based on the allowed origins list.
// nil means same-origin only (no CORS headers), ["*"] means wildcard.
func cors(origins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(origins) > 0 {
			if origins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				origin := r.Header.Get("Origin")
				for _, allowed := range origins {
					if origin == allowed {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// bodyLimit limits the request body size for POST and PUT requests.
func bodyLimit(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// requireCapability wraps a handler to reject requests the running edition does
// not open. The refusal names the capability and the edition that would grant
// it, so the interface can build its message and its upgrade link without
// parsing the text.
func requireCapability(c extension.Capability, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !extension.Allows(c) {
			required := extension.MinEdition(c)
			WriteErrorDetail(w, http.StatusForbidden, ErrorDetail{
				Code:            "EDITION_REQUIRED",
				Message:         "This feature requires the " + titleEdition(required) + " edition.",
				Feature:         string(c),
				RequiredEdition: string(required),
			})
			return
		}
		next(w, r)
	}
}

// resolveHistoryWindow turns a window parameter into a window the running
// edition actually opens, and writes the refusal itself when it cannot.
//
// It is the single place both history endpoints decide from, so they cannot
// drift: a window the product does not know is a bad request, a window it knows
// but the edition does not open is an edition refusal, and the two never look
// alike. Nothing here falls back to the cap: a caller asking for more than it
// gets has to be told, not quietly served less.
func resolveHistoryWindow(w http.ResponseWriter, name, invalidCode string) (extension.HistoryWindow, bool) {
	window, known := extension.ResolveHistoryWindow(name)
	if !known {
		WriteError(w, http.StatusBadRequest, invalidCode,
			"Window must be one of "+extension.HistoryWindowNames())
		return extension.HistoryWindow{}, false
	}

	if allowed, required := extension.AllowsHistoryWindow(window); !allowed {
		WriteErrorDetail(w, http.StatusForbidden, ErrorDetail{
			Code:            "EDITION_REQUIRED",
			Message:         "The " + window.Name + " window requires the " + titleEdition(required) + " edition.",
			Feature:         string(extension.CapResourceHistory),
			RequiredEdition: string(required),
			Window:          window.Name,
			MaxWindow:       extension.MaxHistoryWindow().Name,
		})
		return extension.HistoryWindow{}, false
	}

	return window, true
}

// refuseCapability writes the same refusal as requireCapability, for handlers
// that check a capability partway through rather than at the door.
func refuseCapability(w http.ResponseWriter, c extension.Capability) {
	required := extension.MinEdition(c)
	WriteErrorDetail(w, http.StatusForbidden, ErrorDetail{
		Code:            "EDITION_REQUIRED",
		Message:         "This feature requires the " + titleEdition(required) + " edition.",
		Feature:         string(c),
		RequiredEdition: string(required),
	})
}

// resourceLabel is how a capped resource is named in a refusal message.
var resourceLabel = map[extension.Resource]string{
	extension.ResourceEndpoints:        "endpoints",
	extension.ResourceHeartbeats:       "heartbeat monitors",
	extension.ResourceCertificates:     "certificate monitors",
	extension.ResourceStatusComponents: "status page components",
	extension.ResourceAgentHosts:       "remote agents",
}

// refuseQuota writes the 403 QUOTA_EXCEEDED refusal shared by the four capped
// creation paths. resource, limit and the edition that lifts the cap all come
// from extension.Limit — the interface composes its own sentence from them and
// never parses this text.
func refuseQuota(w http.ResponseWriter, resource extension.Resource) {
	writeQuotaRefusal(w, http.StatusForbidden, "QUOTA_EXCEEDED", resource)
}

func writeQuotaRefusal(w http.ResponseWriter, status int, code string, resource extension.Resource) {
	limit := extension.Limit(resource)
	label := resourceLabel[resource]
	if label == "" {
		label = string(resource)
	}

	// The next edition up is the one that lifts the cap. Personal lifts every
	// cap except agent hosts, which only Pro makes unlimited.
	required := extension.Personal
	if resource == extension.ResourceAgentHosts && extension.CurrentEdition().AtLeast(extension.Personal) {
		required = extension.Pro
	}

	WriteErrorDetail(w, status, ErrorDetail{
		Code: code,
		Message: fmt.Sprintf("The %s edition is limited to %d %s.",
			titleEdition(extension.CurrentEdition()), limit, label),
		Resource:        string(resource),
		Limit:           &limit,
		RequiredEdition: string(required),
	})
}

// titleEdition renders an edition for display: "personal" reads as "Personal".
func titleEdition(e extension.Edition) string {
	s := string(e)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
