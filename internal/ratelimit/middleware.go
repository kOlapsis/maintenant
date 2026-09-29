// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package ratelimit

import "net/http"

// AllowRequest consumes one token for the address the request is attributed to.
func (l *Limiter) AllowRequest(r *http.Request) bool {
	return l.Allow(l.resolver.ClientIP(r))
}

// Middleware wraps an http.Handler and rejects requests that exceed the rate limit
// with a 429 status code.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.AllowRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"Too many requests"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
