// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package ratelimit

import (
	"math"
	"net/http"
	"strconv"
)

// AllowRequest consumes one token for the address the request is attributed to.
func (l *Limiter) AllowRequest(r *http.Request) bool {
	return l.Allow(l.resolver.ClientIP(r))
}

// Reject answers a refused request: 429, the JSON rate_limited error and the seconds until a token is back.
func (l *Limiter) Reject(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(l.retryAfterSeconds()))
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"Too many requests"}}`))
}

// retryAfterSeconds is the time one token takes to refill, never under a second.
func (l *Limiter) retryAfterSeconds() int {
	if l.rate <= 0 {
		return 1
	}
	return max(1, int(math.Ceil(1/l.rate)))
}

// Middleware wraps an http.Handler and rejects requests that exceed the rate limit
// with a 429 status code.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.AllowRequest(r) {
			l.Reject(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
