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

package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/resource"
)

const (
	retentionInterval         = 1 * time.Hour
	retentionCatchUpInterval  = 1 * time.Minute
	retentionBudgetPerTable   = 2 * time.Minute
	defaultRetention          = 90 * 24 * time.Hour // 90 days
	retentionBatchSize        = defaultBatchSize
	archivedRetention         = 30 * 24 * time.Hour  // 30 days
	checkResultRetention      = 30 * 24 * time.Hour  // 30 days
	inactiveEndpointRetention = 30 * 24 * time.Hour  // 30 days
	heartbeatPingRetention    = 30 * 24 * time.Hour  // 30 days
	heartbeatExecRetention    = 30 * 24 * time.Hour  // 30 days
	certCheckResultRetention  = 30 * 24 * time.Hour  // 30 days
	// Raw samples only feed the ranges up to 24h; 7d reads the hourly rollup.
	resourceSnapshotRetention = resource.DefaultSnapshotRetention
	resourceHourlyRetention   = 90 * 24 * time.Hour  // 90 days
	resourceDailyRetention    = 365 * 24 * time.Hour // 1 year

	// Below this many deleted rows an autocheckpoint keeps up on its own and
	// forcing a truncating checkpoint is just noise.
	checkpointRowThreshold = 50000
)

// RetentionConfig tunes the retention cleanup. A zero value means "use the
// defaults", so an empty struct reproduces the historical behaviour.
type RetentionConfig struct {
	// Scheduling
	Interval        time.Duration // between two full passes
	CatchUpInterval time.Duration // used instead of Interval while a backlog remains
	BudgetPerTable  time.Duration // caps how long one table may hold the writer
	BatchSize       int           // rows deleted per transaction

	// Windows
	Snapshots         time.Duration
	Hourly            time.Duration
	Daily             time.Duration
	Transitions       time.Duration
	Archived          time.Duration
	CheckResults      time.Duration
	InactiveEndpoints time.Duration
	HeartbeatPings    time.Duration
	HeartbeatExecs    time.Duration
	CertCheckResults  time.Duration
}

// withDefaults fills unset fields and rejects values that would break the loop
// (an interval shorter than a pass, a batch size that never terminates, a raw
// window too short to feed the 7-day resource graphs).
func (c RetentionConfig) withDefaults(logger *slog.Logger) RetentionConfig {
	c.Interval = clampDuration(logger, "retention interval", c.Interval, retentionInterval, time.Minute)
	c.CatchUpInterval = clampDuration(logger, "retention catch-up interval", c.CatchUpInterval, retentionCatchUpInterval, 10*time.Second)
	c.BudgetPerTable = clampDuration(logger, "retention budget", c.BudgetPerTable, retentionBudgetPerTable, 10*time.Second)
	if c.BudgetPerTable > 30*time.Minute {
		logger.Warn("retention: budget above maximum, clamping", "requested", c.BudgetPerTable, "used", 30*time.Minute)
		c.BudgetPerTable = 30 * time.Minute
	}

	switch {
	case c.BatchSize == 0:
		c.BatchSize = retentionBatchSize
	case c.BatchSize < 100:
		logger.Warn("retention: batch size below minimum, clamping", "requested", c.BatchSize, "used", 100)
		c.BatchSize = 100
	case c.BatchSize > 100000:
		logger.Warn("retention: batch size above maximum, clamping", "requested", c.BatchSize, "used", 100000)
		c.BatchSize = 100000
	}

	// The 24h range groups raw samples, so anything shorter would leave it with
	// holes. Longer ranges read the hourly rollup and are not affected.
	c.Snapshots = clampDuration(logger, "resource snapshot retention", c.Snapshots, resourceSnapshotRetention, 24*time.Hour)

	c.Hourly = orDefault(c.Hourly, resourceHourlyRetention)
	c.Daily = orDefault(c.Daily, resourceDailyRetention)
	c.Transitions = orDefault(c.Transitions, defaultRetention)
	c.Archived = orDefault(c.Archived, archivedRetention)
	c.CheckResults = orDefault(c.CheckResults, checkResultRetention)
	c.InactiveEndpoints = orDefault(c.InactiveEndpoints, inactiveEndpointRetention)
	c.HeartbeatPings = orDefault(c.HeartbeatPings, heartbeatPingRetention)
	c.HeartbeatExecs = orDefault(c.HeartbeatExecs, heartbeatExecRetention)
	c.CertCheckResults = orDefault(c.CertCheckResults, certCheckResultRetention)

	return c
}

func orDefault(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}

func clampDuration(logger *slog.Logger, name string, v, def, floor time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	if v < floor {
		logger.Warn("retention: "+name+" below minimum, clamping", "requested", v, "used", floor)
		return floor
	}
	return v
}

func (c RetentionConfig) batchOpts() batchOpts {
	return batchOpts{batchSize: c.BatchSize, budget: c.BudgetPerTable}
}

// nextDelay decides when the following pass runs. A pass that stopped on its
// budget left rows behind, so waiting a full interval would let the backlog grow
// again — exactly the failure this cleanup is meant to avoid.
func (c RetentionConfig) nextDelay(truncated bool) time.Duration {
	if truncated {
		return c.CatchUpInterval
	}
	return c.Interval
}

// RetentionOpts holds optional stores for retention cleanup.
type RetentionOpts struct {
	EndpointStore    *EndpointStore
	HeartbeatStore   *HeartbeatStore
	CertificateStore *CertificateStore
	ResourceStore    *ResourceStore
	Config           RetentionConfig
}

// retentionPass accumulates what one pass did, so the scheduler knows whether a
// backlog remains and the vacuum knows whether it is worth running.
type retentionPass struct {
	deleted   int64
	truncated bool
}

func (p *retentionPass) add(deleted int64, truncated bool) {
	p.deleted += deleted
	p.truncated = p.truncated || truncated
}

// StartRetentionCleanupWithOpts starts retention cleanup with all store types.
// The first pass runs immediately: an instance that restarts every few hours
// would otherwise never reach its first cleanup.
func StartRetentionCleanupWithOpts(ctx context.Context, store *ContainerStore, db *DB, logger *slog.Logger, opts RetentionOpts) {
	cfg := opts.Config.withDefaults(logger)
	logger.Info("retention cleanup: starting",
		"interval", cfg.Interval.String(),
		"batch_size", cfg.BatchSize,
		"snapshot_window", cfg.Snapshots.String())

	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			truncated := runRetentionPass(ctx, store, db, logger, opts, cfg)
			timer.Reset(cfg.nextDelay(truncated))
		}
	}()
}

// runRetentionPass runs every cleanup once and reports whether rows matching the
// retention windows are still present.
func runRetentionPass(ctx context.Context, store *ContainerStore, db *DB, logger *slog.Logger, opts RetentionOpts, cfg RetentionConfig) bool {
	started := time.Now()
	var pass retentionPass

	runCleanup(ctx, store, logger, cfg, &pass)
	if opts.EndpointStore != nil {
		runEndpointCleanup(ctx, opts.EndpointStore, logger, cfg, &pass)
	}
	if opts.HeartbeatStore != nil {
		runHeartbeatCleanup(ctx, opts.HeartbeatStore, logger, cfg, &pass)
	}
	if opts.CertificateStore != nil {
		runCertificateCleanup(ctx, opts.CertificateStore, logger, cfg, &pass)
	}
	if opts.ResourceStore != nil {
		runResourceCleanup(ctx, opts.ResourceStore, logger, cfg, &pass)
	}

	if pass.deleted > 0 {
		logger.Info("retention cleanup: pass complete",
			"deleted", pass.deleted,
			"duration", time.Since(started).Round(time.Millisecond).String(),
			"backlog_remaining", pass.truncated)
		reclaimSpace(ctx, db, logger, cfg.BudgetPerTable, pass.deleted)
	}
	if pass.truncated {
		logger.Warn("retention cleanup: budget exhausted, resuming shortly",
			"next_pass_in", cfg.CatchUpInterval.String())
	}
	return pass.truncated
}

func runCleanup(ctx context.Context, store *ContainerStore, logger *slog.Logger, cfg RetentionConfig, pass *retentionPass) {
	// Clean old transitions
	cutoff := time.Now().Add(-cfg.Transitions)
	deleted, truncated, err := store.deleteTransitionsBefore(ctx, cutoff, cfg.batchOpts())
	pass.add(deleted, truncated)
	if err != nil {
		logger.Error("retention cleanup: transitions", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted transitions", "count", deleted)
	}

	// Clean old archived containers
	archiveCutoff := time.Now().Add(-cfg.Archived)
	archivedDeleted, err := store.DeleteArchivedContainersBefore(ctx, archiveCutoff)
	pass.add(archivedDeleted, false)
	if err != nil {
		logger.Error("retention cleanup: archived containers", "error", err)
	} else if archivedDeleted > 0 {
		logger.Info("retention cleanup: deleted archived containers", "count", archivedDeleted)
	}
}

func runHeartbeatCleanup(ctx context.Context, store *HeartbeatStore, logger *slog.Logger, cfg RetentionConfig, pass *retentionPass) {
	// Clean old heartbeat pings
	pingCutoff := time.Now().Add(-cfg.HeartbeatPings)
	deleted, truncated, err := store.deletePingsBefore(ctx, pingCutoff, cfg.batchOpts())
	pass.add(deleted, truncated)
	if err != nil {
		logger.Error("retention cleanup: heartbeat pings", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted heartbeat pings", "count", deleted)
	}

	// Clean old heartbeat executions
	execCutoff := time.Now().Add(-cfg.HeartbeatExecs)
	execDeleted, execTruncated, err := store.deleteExecutionsBefore(ctx, execCutoff, cfg.batchOpts())
	pass.add(execDeleted, execTruncated)
	if err != nil {
		logger.Error("retention cleanup: heartbeat executions", "error", err)
	} else if execDeleted > 0 {
		logger.Info("retention cleanup: deleted heartbeat executions", "count", execDeleted)
	}
}

func runCertificateCleanup(ctx context.Context, store *CertificateStore, logger *slog.Logger, cfg RetentionConfig, pass *retentionPass) {
	cutoff := time.Now().Add(-cfg.CertCheckResults)
	deleted, truncated, err := store.deleteCheckResultsBefore(ctx, cutoff, cfg.batchOpts())
	pass.add(deleted, truncated)
	if err != nil {
		logger.Error("retention cleanup: cert check results", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted cert check results", "count", deleted)
	}
}

func runResourceCleanup(ctx context.Context, store *ResourceStore, logger *slog.Logger, cfg RetentionConfig, pass *retentionPass) {
	cutoff := time.Now().Add(-cfg.Snapshots)
	deleted, truncated, err := store.deleteSnapshotsBefore(ctx, cutoff, cfg.batchOpts())
	pass.add(deleted, truncated)
	if err != nil {
		logger.Error("retention cleanup: resource snapshots", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted resource snapshots", "count", deleted)
	}

	hourlyCutoff := time.Now().Add(-cfg.Hourly)
	hourlyDeleted, hourlyTruncated, err := store.deleteHourlyBefore(ctx, hourlyCutoff, cfg.batchOpts())
	pass.add(hourlyDeleted, hourlyTruncated)
	if err != nil {
		logger.Error("retention cleanup: resource hourly", "error", err)
	} else if hourlyDeleted > 0 {
		logger.Info("retention cleanup: deleted resource hourly", "count", hourlyDeleted)
	}

	dailyCutoff := time.Now().Add(-cfg.Daily)
	dailyDeleted, dailyTruncated, err := store.deleteDailyBefore(ctx, dailyCutoff, cfg.batchOpts())
	pass.add(dailyDeleted, dailyTruncated)
	if err != nil {
		logger.Error("retention cleanup: resource daily", "error", err)
	} else if dailyDeleted > 0 {
		logger.Info("retention cleanup: deleted resource daily", "count", dailyDeleted)
	}
}

func runEndpointCleanup(ctx context.Context, store *EndpointStore, logger *slog.Logger, cfg RetentionConfig, pass *retentionPass) {
	// Clean old check results
	cutoff := time.Now().Add(-cfg.CheckResults)
	deleted, truncated, err := store.deleteCheckResultsBefore(ctx, cutoff, cfg.batchOpts())
	pass.add(deleted, truncated)
	if err != nil {
		logger.Error("retention cleanup: check results", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted check results", "count", deleted)
	}

	// Clean inactive endpoints
	inactiveCutoff := time.Now().Add(-cfg.InactiveEndpoints)
	epDeleted, err := store.DeleteInactiveEndpointsBefore(ctx, inactiveCutoff)
	pass.add(epDeleted, false)
	if err != nil {
		logger.Error("retention cleanup: inactive endpoints", "error", err)
	} else if epDeleted > 0 {
		logger.Info("retention cleanup: deleted inactive endpoints", "count", epDeleted)
	}
}

// reclaimSpace returns the pages freed by the pass to the filesystem and keeps
// the WAL from carrying the whole reclaim at once. On a database whose header
// says auto_vacuum NONE there is nothing to reclaim without a full VACUUM, so
// only the checkpoint runs — Open already told the operator why.
func reclaimSpace(ctx context.Context, db *DB, logger *slog.Logger, budget time.Duration, deleted int64) {
	if db.incrementalVacuum {
		incrementalVacuum(ctx, db, logger, budget)
	}
	if deleted >= checkpointRowThreshold {
		walCheckpoint(ctx, db, logger, "TRUNCATE")
	}
}

func incrementalVacuum(ctx context.Context, db *DB, logger *slog.Logger, budget time.Duration) {
	before, err := freelistCount(ctx, db)
	if err != nil {
		logger.Debug("retention cleanup: freelist count", "error", err)
		return
	}

	remaining := before
	deadline := time.Now().Add(budget)
	for remaining > 0 && time.Now().Before(deadline) && ctx.Err() == nil {
		// incremental_vacuum is a write: it must go through the serialized
		// writer, or it races the writer goroutine for the write lock.
		if _, err := db.Writer().Exec(ctx, "PRAGMA incremental_vacuum(2000)"); err != nil {
			logger.Error("retention cleanup: incremental vacuum", "error", err)
			return
		}
		// Moved pages travel through the WAL, so checkpoint between slices
		// instead of reclaiming everything into an ever-growing WAL.
		walCheckpoint(ctx, db, logger, "PASSIVE")

		next, err := freelistCount(ctx, db)
		if err != nil {
			logger.Debug("retention cleanup: freelist count", "error", err)
			return
		}
		if next >= remaining {
			break
		}
		remaining = next
	}

	if before > remaining {
		logger.Info("retention cleanup: reclaimed pages", "freed", before-remaining, "remaining", remaining)
	}
}

func freelistCount(ctx context.Context, db *DB) (int, error) {
	var pages int
	err := db.ReadDB().QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&pages)
	return pages, err
}

// walCheckpoint is best effort: busy=1 simply means readers were active, which
// is the common case here since the resource rollup reads continuously.
func walCheckpoint(ctx context.Context, db *DB, logger *slog.Logger, mode string) {
	var busy, walPages, checkpointed int
	// mode is a package-level literal, never user input.
	err := db.ReadDB().QueryRowContext(ctx, fmt.Sprintf("PRAGMA wal_checkpoint(%s)", mode)).
		Scan(&busy, &walPages, &checkpointed)
	if err != nil {
		logger.Debug("retention cleanup: wal checkpoint failed", "mode", mode, "error", err)
		return
	}
	logger.Debug("retention cleanup: wal checkpoint",
		"mode", mode, "busy", busy, "wal_pages", walPages, "checkpointed", checkpointed)
}
