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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secretPassword = "s3cr3t-Sentinel%40!"

func TestParseDSN(t *testing.T) {
	u, err := ParseDSN("postgres://app:pw@db.internal:5432/maintenant?sslmode=require")
	require.NoError(t, err)
	assert.Equal(t, "db.internal:5432", u.Host)

	_, err = ParseDSN("postgresql://app@localhost/maintenant")
	assert.NoError(t, err)

	for name, raw := range map[string]string{
		"empty":        "",
		"blank":        "   ",
		"wrong scheme": "mysql://app@db/maintenant",
		"no scheme":    "host=db user=app dbname=maintenant",
		"malformed":    "postgres://app:pw@db:port%%/x",
	} {
		_, err := ParseDSN(raw)
		assert.ErrorIs(t, err, ErrInvalidDSN, name)
		if err != nil {
			assert.NotContains(t, err.Error(), "pw", name)
		}
	}
}

func TestApplyDefaultSSLMode(t *testing.T) {
	// Absent + remote host: require is added (FR-022).
	got := ApplyDefaultSSLMode("postgres://app:pw@db.internal:5432/maintenant")
	assert.Contains(t, got, "sslmode=require")

	// Absent + local host: untouched.
	for _, raw := range []string{
		"postgres://app@localhost:5432/maintenant",
		"postgres://app@127.0.0.1/maintenant",
		"postgres://app@[::1]:5432/maintenant",
		"postgres://app@/maintenant?host=/var/run/postgresql",
	} {
		assert.Equal(t, raw, ApplyDefaultSSLMode(raw), raw)
	}

	// Explicit value, disable included: always wins, even remote.
	for _, raw := range []string{
		"postgres://app@db.internal/maintenant?sslmode=disable",
		"postgres://app@db.internal/maintenant?sslmode=verify-full",
	} {
		assert.Equal(t, raw, ApplyDefaultSSLMode(raw), raw)
	}

	// Unparseable: returned unchanged, the open fails with ErrInvalidDSN.
	assert.Equal(t, "not-a-dsn", ApplyDefaultSSLMode("not-a-dsn"))
}

func TestRedactDSN(t *testing.T) {
	raw := "postgres://maintenant:" + secretPassword + "@db.internal:5432/prod?sslmode=require&application_name=x"
	red := RedactDSN(raw)
	assert.Equal(t, "postgres://maintenant@db.internal:5432/prod", red)
	assert.NotContains(t, red, secretPassword)

	// No user info at all.
	assert.Equal(t, "postgres://db.internal/prod", RedactDSN("postgres://db.internal/prod"))

	// Unparseable input: nothing of it is echoed (SC-007).
	red = RedactDSN("::::" + secretPassword)
	assert.Equal(t, redactedInvalid, red)
	assert.NotContains(t, red, secretPassword)
}

// TestNoOutputContainsPassword sweeps every string this file can produce from
// a DSN carrying a sentinel password (SC-007, first half).
func TestNoOutputContainsPassword(t *testing.T) {
	raw := "postgres://app:" + secretPassword + "@db.internal:5432/maintenant"
	outputs := []string{
		RedactDSN(raw),
		DSNHost(raw),
		DSNDatabase(raw),
	}
	if _, err := ParseDSN("mysql://app:" + secretPassword + "@db/x"); err != nil {
		outputs = append(outputs, err.Error())
	}
	for _, out := range outputs {
		assert.NotContains(t, out, secretPassword)
	}
}

func TestDSNHostAndDatabase(t *testing.T) {
	raw := "postgres://app:pw@db.internal:5432/maintenant?sslmode=require"
	assert.Equal(t, "db.internal:5432", DSNHost(raw))
	assert.Equal(t, "maintenant", DSNDatabase(raw))
	assert.Empty(t, DSNHost("garbage"))
	assert.Empty(t, DSNDatabase("garbage"))
}
