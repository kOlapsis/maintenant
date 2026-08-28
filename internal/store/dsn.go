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

package store

import (
	"fmt"
	"net/url"
	"strings"
)

// redactedInvalid is what RedactDSN returns when the string cannot be parsed:
// an unparseable DSN may still contain a password, so none of it is echoed.
const redactedInvalid = "(invalid connection string)"

// ParseDSN validates a PostgreSQL connection URL. Only the postgres:// and
// postgresql:// schemes are accepted; anything else wraps ErrInvalidDSN.
func ParseDSN(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidDSN)
	}
	u, err := url.Parse(raw)
	if err != nil {
		// The parse error would echo the raw string; keep it out.
		return nil, fmt.Errorf("%w: not a valid URL", ErrInvalidDSN)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("%w: scheme %q (want postgres:// or postgresql://)", ErrInvalidDSN, u.Scheme)
	}
	return u, nil
}

// localDSNHost reports whether the DSN points at the local machine: a loopback
// hostname, an empty host, or a Unix-socket path (empty URL host with a `host`
// query parameter starting with '/').
func localDSNHost(u *url.URL) bool {
	h := u.Hostname()
	if h == "" {
		return true
	}
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	return strings.HasPrefix(u.Query().Get("host"), "/")
}

// ApplyDefaultSSLMode enforces the product's one TLS default (FR-022): when
// the connection string carries no explicit sslmode and the host is not local,
// sslmode=require is added. An explicit value, disable included, always wins.
// An unparseable string is returned unchanged; opening it fails with
// ErrInvalidDSN anyway.
func ApplyDefaultSSLMode(raw string) string {
	u, err := ParseDSN(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Has("sslmode") || localDSNHost(u) {
		return raw
	}
	q.Set("sslmode", "require")
	u.RawQuery = q.Encode()
	return u.String()
}

// RedactDSN renders a connection string safe for logs and errors:
// scheme://user@host:port/database, no password, no parameters. Principle VI
// and FR-021: credentials never reach logs, responses or telemetry.
func RedactDSN(raw string) string {
	u, err := ParseDSN(raw)
	if err != nil {
		return redactedInvalid
	}
	r := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}
	if name := u.User.Username(); name != "" {
		r.User = url.User(name)
	}
	return r.String()
}

// DSNHost returns the host (with port when present) for the startup log line.
func DSNHost(raw string) string {
	u, err := ParseDSN(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// DSNDatabase returns the database name for the startup log line.
func DSNDatabase(raw string) string {
	u, err := ParseDSN(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}
