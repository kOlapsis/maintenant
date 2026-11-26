// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/swarm"
	"github.com/kolapsis/maintenant/internal/update"
)

// TestTx_CallerCancelledMidTransactionSharesNothingWithIt fails through the race detector only.
func TestTx_CallerCancelledMidTransactionSharesNothingWithIt(t *testing.T) {
	requireSQLite(t)
	now := time.Now()
	swarmNode := func() *swarm.SwarmNode {
		return &swarm.SwarmNode{
			AgentID: "swarm-A", NodeID: "n1", Hostname: "n1", Role: "manager", Status: "ready", Availability: "active",
			FirstSeenAt: now, LastSeenAt: now, LastStatusChangeAt: now,
		}
	}
	archived := func(t *testing.T, db *DB) *ContainerStore {
		cs := NewContainerStore(db)
		seedAgent(t, db, "agent-staging")
		id := seedHostedContainer(t, cs, "agent-staging", "old", "old", "app:1")
		require.NoError(t, cs.ArchiveContainer(context.Background(), id, now.Add(-48*time.Hour)))
		return cs
	}
	countWithError := func(deleted int64, err error) error {
		if err != nil && deleted != 0 {
			return errors.New("a count came back with the error")
		}
		return err
	}

	cases := map[string]func(t *testing.T, db *DB) func(ctx context.Context) error{
		"DeleteArchivedContainersBefore": func(t *testing.T, db *DB) func(ctx context.Context) error {
			cs := archived(t, db)
			return func(ctx context.Context) error {
				return countWithError(cs.DeleteArchivedContainersBefore(ctx, now))
			}
		},
		"DeleteOldestArchivedContainers": func(t *testing.T, db *DB) func(ctx context.Context) error {
			cs := archived(t, db)
			return func(ctx context.Context) error {
				return countWithError(cs.DeleteOldestArchivedContainers(ctx, "agent-staging", 5))
			}
		},
		"CreateExclusion": func(_ *testing.T, db *DB) func(ctx context.Context) error {
			us := NewUpdateStore(db)
			return func(ctx context.Context) error {
				e := &update.UpdateExclusion{Pattern: "nginx*", PatternType: update.ExclusionTypeImage, CreatedAt: now}
				_, err := us.CreateExclusion(ctx, e)
				if err != nil && e.ID != "" {
					return errors.New("the exclusion got an id with the error")
				}
				return err
			}
		},
		"UpsertNode": func(_ *testing.T, db *DB) func(ctx context.Context) error {
			s := NewSwarmNodeStore(db)
			return func(ctx context.Context) error {
				n := swarmNode()
				err := s.UpsertNode(ctx, n)
				if n.ID == "" {
					return errors.New("the node has no id")
				}
				return err
			}
		},
		"ReplaceNodesForAgent": func(_ *testing.T, db *DB) func(ctx context.Context) error {
			s := NewSwarmNodeStore(db)
			return func(ctx context.Context) error {
				n := swarmNode()
				err := s.ReplaceNodesForAgent(ctx, "swarm-A", []*swarm.SwarmNode{n})
				if n.ID == "" {
					return errors.New("the node has no id")
				}
				return err
			}
		},
	}

	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			bg := context.Background()
			db := openTestDB(t)
			call := setup(t, db)
			w := db.Writer()

			started, release := make(chan struct{}), make(chan struct{})
			held := make(chan error, 1)
			go func() {
				held <- w.Tx(bg, func(context.Context, *Tx) error {
					close(started)
					<-release
					return nil
				})
			}()
			<-started

			ctx, cancel := context.WithCancel(bg)
			returned := make(chan error, 1)
			go func() { returned <- call(ctx) }()
			require.Eventually(t, func() bool { return len(w.ch) == 1 }, 5*time.Second, time.Millisecond)

			// Releasing the writer only once the call returned would order the two for the race detector and hide the race.
			cancel()
			runtime.Gosched()
			close(release)

			if err := <-returned; err != nil {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.NoError(t, <-held)
			require.NoError(t, w.Tx(bg, func(context.Context, *Tx) error { return nil }), "the abandoned transaction ends before the test does")
		})
	}
}
