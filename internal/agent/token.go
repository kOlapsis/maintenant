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

package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// TokenPrefixLen is how much of a token is kept in the clear for display. The
// prefix is `mnt_enr_` plus six base32 characters: enough for an operator to
// tell two tokens apart in a list, and 30 bits out of 256 — worthless to
// someone holding only the hash, which is the whole point of storing it.
const TokenPrefixLen = 14

// tokenScheme prefixes every enrollment token so one is recognisable on sight
// in a log, a shell history or a support paste.
const tokenScheme = "mnt_enr_" // #nosec G101 -- a fixed scheme prefix, not a credential.

// NewToken mints an enrollment token and returns its cleartext, the hash that
// gets persisted, the id derived from that hash, and the display prefix.
//
// The cleartext is returned once and never stored: it exists in the creation
// response and nowhere else. Everything the server needs afterwards to accept,
// list or revoke the token is derivable from the hash.
func NewToken() (cleartext, hash, id, prefix string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", "", "", err
	}
	cleartext = tokenScheme + strings.ToLower(strings.TrimRight(
		base32.StdEncoding.EncodeToString(raw), "="))
	hash = HashToken(cleartext)
	return cleartext, hash, TokenIDFromHash(hash), TokenPrefix(cleartext), nil
}

// HashToken returns the hex-encoded SHA-256 of an enrollment token. This is the
// only form of the token the database ever sees.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// TokenIDFromHash derives the opaque id used in API paths from the token hash.
// Truncating is safe: the id identifies a row, it never authorises anything —
// enrollment matches on the full hash.
func TokenIDFromHash(hash string) string {
	if len(hash) < 16 {
		return hash
	}
	return hash[:16]
}

// TokenPrefix returns the leading, non-secret part of a token.
func TokenPrefix(token string) string {
	if len(token) <= TokenPrefixLen {
		return token
	}
	return token[:TokenPrefixLen]
}

// Masked renders a token for display from the stored prefix alone. The rest is
// unrecoverable by design, so this is all any read path can ever show.
func (t *EnrollmentToken) Masked() string {
	return t.TokenPrefix + "...***"
}
