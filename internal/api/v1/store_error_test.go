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

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteStoreError distinguishes an outage the operator waits out from a
// failure they must act on. The interface can explain and retry a 503; a 500
// leaves it with an empty screen and no reason (FR-023).
func TestWriteStoreError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
		wantKey  string
	}{
		{
			name:     "network failure is an outage",
			err:      fmt.Errorf("list containers: %w", &net.OpError{Op: "read", Err: errors.New("connection reset")}),
			wantCode: http.StatusServiceUnavailable,
			wantKey:  "STORAGE_UNAVAILABLE",
		},
		{
			name:     "too many connections is an outage",
			err:      fmt.Errorf("list containers: %w", &pgconn.PgError{Code: "53300"}),
			wantCode: http.StatusServiceUnavailable,
			wantKey:  "STORAGE_UNAVAILABLE",
		},
		{
			name:     "a constraint violation is not an outage",
			err:      fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505"}),
			wantCode: http.StatusInternalServerError,
			wantKey:  "INTERNAL_ERROR",
		},
		{
			name:     "an ordinary error is not an outage",
			err:      errors.New("malformed row"),
			wantCode: http.StatusInternalServerError,
			wantKey:  "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			WriteStoreError(w, tt.err, "Failed to list containers")

			assert.Equal(t, tt.wantCode, w.Code)

			var body ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.wantKey, body.Error.Code)

			// Whatever happened, the response never carries the driver's own
			// wording, which can name the host or the database.
			assert.NotContains(t, w.Body.String(), "connection reset")
		})
	}
}
