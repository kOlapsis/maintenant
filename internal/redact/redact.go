// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package redact cuts what can carry a credential out of the URLs shown outside an edit form.
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

// URL returns raw reduced to its scheme, host and port when it is a URL, and raw unchanged otherwise.
func URL(raw string) string {
	i := strings.Index(raw, "://")
	if i < 0 {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw[:i]) + "://"
	}
	return u.Scheme + "://" + u.Host
}

var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'\\<>]+`)

// Text returns msg with every URL it quotes reduced as URL does.
func Text(msg string) string {
	return urlInText.ReplaceAllStringFunc(msg, URL)
}
