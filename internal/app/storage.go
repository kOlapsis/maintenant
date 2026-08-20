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

package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

const (
	// instanceBeatInterval is how often this instance refreshes its row.
	instanceBeatInterval = 30 * time.Second
	// instancePeerWindow is how recently another instance must have beaten to
	// count as a live peer.
	instancePeerWindow = 2 * time.Minute
	// instanceStaleAfter is when a silent instance stops being reported.
	instanceStaleAfter = 5 * time.Minute
	// instancePurgeInterval is how often stale rows are dropped.
	instancePurgeInterval = 5 * time.Minute
)

// openStorage opens the configured storage. Absent a connection string it is
// SQLite on cfg.DBPath, exactly as before; with one it is the operator's
// PostgreSQL, and a failure stops the process rather than falling back
// (FR-001, FR-004). Errors keep the startup sentinels so main can name the
// cause, and never carry the connection string (FR-021).
func openStorage(ctx context.Context, cfg Config, logger *slog.Logger) (*store.DB, error) {
	if cfg.DatabaseURL == "" {
		db, err := store.Open(cfg.DBPath, logger)
		if err != nil {
			return nil, fmt.Errorf("open database: %w", err)
		}
		return db, nil
	}
	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}

// logStorageOpened emits the one startup line naming the engine, and on
// PostgreSQL its host, database and schema version. The SQLite path stays
// silent here: Open already logs the storage mode, and an unconfigured
// install must not see a new message (US3).
func logStorageOpened(ctx context.Context, db *store.DB, logger *slog.Logger) {
	if db.Dialect() == store.DialectSQLite {
		return
	}
	version, err := db.SchemaVersion(ctx)
	if err != nil {
		logger.Warn("storage opened, schema version unreadable", "engine", db.Engine(), "error", err)
		return
	}
	logger.Info("storage opened",
		"engine", db.Engine(),
		"host", db.Host(),
		"database", db.Database(),
		"schema_version", version)
}

// storageProbeTTL is how stale a liveness answer may be before Connected
// probes again. Once the storage supervisor runs it refreshes the flag well
// within this window, so the lazy probe never fires in practice.
const storageProbeTTL = 10 * time.Second

// storageProbeTimeout bounds the lazy probe so a health request cannot hang
// on an unreachable database.
const storageProbeTimeout = 3 * time.Second

// storageState is the health view of the storage: which engine, whether it
// answers, and how many other instances beat on the same database.
type storageState struct {
	engine string
	db     *store.DB

	connected atomic.Bool
	peers     atomic.Int64
	// probedAtUnixNano stamps the last liveness answer, whoever produced it.
	probedAtUnixNano atomic.Int64
}

func (s *storageState) Engine() string { return s.engine }

// Connected reports whether the database answers. It serves the last known
// answer while it is fresh, and probes when it is not — so the health
// diagnostic never asserts a state it cannot back.
func (s *storageState) Connected() bool {
	last := time.Unix(0, s.probedAtUnixNano.Load())
	if time.Since(last) < storageProbeTTL {
		return s.connected.Load()
	}
	return s.probe(context.Background())
}

// probe pings the database and records the answer. The storage supervisor
// calls it on its own schedule; Connected calls it when its answer is stale.
func (s *storageState) probe(ctx context.Context) bool {
	if s.db == nil {
		return s.connected.Load()
	}
	ctx, cancel := context.WithTimeout(ctx, storageProbeTimeout)
	defer cancel()
	ok := s.db.PingContext(ctx) == nil
	s.connected.Store(ok)
	s.probedAtUnixNano.Store(time.Now().UnixNano())
	return ok
}

func (s *storageState) Peers() int { return int(s.peers.Load()) }

// registerInstance records this process in the instances table and takes the
// first peer census. The table informs, it never arbitrates: no lock, no
// lease, no election (FR-012, FR-013).
func (a *App) registerInstance(ctx context.Context, db *store.DB) error {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	now := time.Now()

	a.instanceStore = store.NewInstanceStore(db)
	a.instanceID = uid.New()
	a.storage = &storageState{engine: db.Engine(), db: db}
	a.storage.connected.Store(true)
	a.storage.probedAtUnixNano.Store(time.Now().UnixNano())

	// The writer goroutine is not started yet on SQLite; register through the
	// pool directly is not possible, so registration happens in Start.
	a.instanceRecord = store.Instance{
		ID:         a.instanceID,
		Hostname:   hostname,
		Version:    a.cfg.Version,
		StartedAt:  now,
		LastSeenAt: now,
	}
	return nil
}

// startInstanceHeartbeat registers this instance, reports any peer already
// beating on the same database, then keeps its own beat and purges the rows
// of instances that stopped beating. Reporting is the only action the product
// takes about a second instance: the operator's cluster manager owns exclusion.
func (a *App) startInstanceHeartbeat(ctx context.Context) {
	if a.instanceStore == nil {
		return
	}
	if err := a.instanceStore.Register(ctx, a.instanceRecord); err != nil {
		a.logger.Warn("instance registration failed, peer visibility is degraded", "error", err)
		return
	}
	a.reportPeers(ctx)

	go func() {
		beat := time.NewTicker(instanceBeatInterval)
		purge := time.NewTicker(instancePurgeInterval)
		defer beat.Stop()
		defer purge.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-beat.C:
				if err := a.instanceStore.Beat(ctx, a.instanceID, time.Now()); err != nil {
					a.logger.Debug("instance heartbeat failed", "error", err)
					continue
				}
				a.reportPeers(ctx)
			case <-purge.C:
				if _, err := a.instanceStore.PurgeStale(ctx, time.Now().Add(-instanceStaleAfter)); err != nil {
					a.logger.Debug("stale instance purge failed", "error", err)
				}
			}
		}
	}()
}

// reportPeers refreshes the peer count and says so, once per transition into
// a peered state, so the operator can see two instances on one database
// instead of discovering it through its effects.
func (a *App) reportPeers(ctx context.Context) {
	peers, err := a.instanceStore.Peers(ctx, a.instanceID, time.Now().Add(-instancePeerWindow))
	if err != nil {
		a.logger.Debug("peer instance census failed", "error", err)
		return
	}
	previous := a.storage.peers.Swap(int64(len(peers)))
	if len(peers) == 0 || previous == int64(len(peers)) {
		return
	}
	hosts := make([]string, 0, len(peers))
	versions := make([]string, 0, len(peers))
	for _, p := range peers {
		hosts = append(hosts, p.Hostname)
		versions = append(versions, p.Version)
	}
	a.logger.Warn("another instance is working on this database",
		"peers", len(peers), "hosts", hosts, "versions", versions,
		"note", "exclusion belongs to your cluster manager; this instance does not arbitrate")
}
