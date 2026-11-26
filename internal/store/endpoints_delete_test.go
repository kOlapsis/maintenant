// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
)

func TestEndpointStore_DeletionTakesTheCheckResultsOrNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		del  func(s *EndpointStore, id string) error
	}{
		{"endpoint", func(s *EndpointStore, id string) error {
			return s.DeleteEndpoint(context.Background(), id)
		}},
		{"standalone", func(s *EndpointStore, id string) error {
			return s.DeleteStandaloneEndpoint(context.Background(), id)
		}},
		{"inactive", func(s *EndpointStore, id string) error {
			_, err := s.DeleteInactiveEndpointsBefore(context.Background(), time.Now().Add(-time.Hour))
			return err
		}},
	} {
		seed := func(t *testing.T) (*DB, *EndpointStore, string) {
			db := openTestDB(t)
			ctx := context.Background()
			s := NewEndpointStore(db)
			id, err := s.InsertStandaloneEndpoint(ctx, &endpoint.Endpoint{
				Name: "site", Target: "https://example.com", EndpointType: endpoint.TypeHTTP,
			})
			require.NoError(t, err)
			_, err = s.InsertCheckResult(ctx, &endpoint.CheckResult{EndpointID: id, Success: true, Timestamp: time.Now()})
			require.NoError(t, err)
			_, err = db.db.Exec(db.dialect.Rebind(`UPDATE endpoints SET active=0, last_seen_at=? WHERE id=?`),
				time.Now().Add(-48*time.Hour).Unix(), id)
			require.NoError(t, err)
			return db, s, id
		}

		t.Run(tc.name+"/deleted", func(t *testing.T) {
			db, s, id := seed(t)
			require.NoError(t, tc.del(s, id))
			assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM endpoints`))
			assert.Zero(t, countAtomicRows(t, db, `SELECT COUNT(*) FROM check_results`))
		})

		t.Run(tc.name+"/failed", func(t *testing.T) {
			db, s, id := seed(t)
			failDeletes(t, db, "endpoints")
			require.Error(t, tc.del(s, id))
			assert.Equal(t, 1, countAtomicRows(t, db, `SELECT COUNT(*) FROM check_results`))
		})
	}
}
