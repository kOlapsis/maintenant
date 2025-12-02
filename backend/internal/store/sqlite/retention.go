package sqlite

import (
	"context"
	"log/slog"
	"time"
)

const (
	retentionInterval         = 1 * time.Hour
	defaultRetention          = 90 * 24 * time.Hour // 90 days
	retentionBatchSize        = 1000
	archivedRetention         = 30 * 24 * time.Hour // 30 days
	checkResultRetention      = 30 * 24 * time.Hour // 30 days
	inactiveEndpointRetention  = 30 * 24 * time.Hour // 30 days
	heartbeatPingRetention     = 30 * 24 * time.Hour // 30 days
	heartbeatExecRetention     = 30 * 24 * time.Hour // 30 days
	certCheckResultRetention   = 30 * 24 * time.Hour // 30 days
	resourceSnapshotRetention  = 7 * 24 * time.Hour  // 7 days
)

// StartRetentionCleanup starts a background goroutine that periodically
// cleans up old state transitions, archived containers, check results, and inactive endpoints.
// RetentionOpts holds optional stores for retention cleanup.
type RetentionOpts struct {
	EndpointStore    *EndpointStore
	HeartbeatStore   *HeartbeatStore
	CertificateStore *CertificateStore
	ResourceStore    *ResourceStore
}

// StartRetentionCleanupWithOpts starts retention cleanup with all store types.
func StartRetentionCleanupWithOpts(ctx context.Context, store *ContainerStore, db *DB, logger *slog.Logger, opts RetentionOpts) {
	go func() {
		ticker := time.NewTicker(retentionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runCleanup(ctx, store, db, logger)
				if opts.EndpointStore != nil {
					runEndpointCleanup(ctx, opts.EndpointStore, logger)
				}
				if opts.HeartbeatStore != nil {
					runHeartbeatCleanup(ctx, opts.HeartbeatStore, logger)
				}
				if opts.CertificateStore != nil {
					runCertificateCleanup(ctx, opts.CertificateStore, logger)
				}
				if opts.ResourceStore != nil {
					runResourceCleanup(ctx, opts.ResourceStore, logger)
				}
			}
		}
	}()
}

func runCleanup(ctx context.Context, store *ContainerStore, db *DB, logger *slog.Logger) {
	// Clean old transitions
	cutoff := time.Now().Add(-defaultRetention)
	deleted, err := store.DeleteTransitionsBefore(ctx, cutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: transitions", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted transitions", "count", deleted)
	}

	// Clean old archived containers
	archiveCutoff := time.Now().Add(-archivedRetention)
	archivedDeleted, err := store.DeleteArchivedContainersBefore(ctx, archiveCutoff)
	if err != nil {
		logger.Error("retention cleanup: archived containers", "error", err)
	} else if archivedDeleted > 0 {
		logger.Info("retention cleanup: deleted archived containers", "count", archivedDeleted)
	}

	// Incremental vacuum
	if deleted > 0 || archivedDeleted > 0 {
		if _, err := db.ReadDB().ExecContext(ctx, "PRAGMA incremental_vacuum(1000)"); err != nil {
			logger.Error("retention cleanup: incremental vacuum", "error", err)
		}
	}
}

func runHeartbeatCleanup(ctx context.Context, store *HeartbeatStore, logger *slog.Logger) {
	// Clean old heartbeat pings
	pingCutoff := time.Now().Add(-heartbeatPingRetention)
	deleted, err := store.DeletePingsBefore(ctx, pingCutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: heartbeat pings", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted heartbeat pings", "count", deleted)
	}

	// Clean old heartbeat executions
	execCutoff := time.Now().Add(-heartbeatExecRetention)
	execDeleted, err := store.DeleteExecutionsBefore(ctx, execCutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: heartbeat executions", "error", err)
	} else if execDeleted > 0 {
		logger.Info("retention cleanup: deleted heartbeat executions", "count", execDeleted)
	}
}

func runCertificateCleanup(ctx context.Context, store *CertificateStore, logger *slog.Logger) {
	cutoff := time.Now().Add(-certCheckResultRetention)
	deleted, err := store.DeleteCheckResultsBefore(ctx, cutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: cert check results", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted cert check results", "count", deleted)
	}
}

func runResourceCleanup(ctx context.Context, store *ResourceStore, logger *slog.Logger) {
	cutoff := time.Now().Add(-resourceSnapshotRetention)
	deleted, err := store.DeleteSnapshotsBefore(ctx, cutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: resource snapshots", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted resource snapshots", "count", deleted)
	}
}

func runEndpointCleanup(ctx context.Context, store *EndpointStore, logger *slog.Logger) {
	// Clean old check results
	cutoff := time.Now().Add(-checkResultRetention)
	deleted, err := store.DeleteCheckResultsBefore(ctx, cutoff, retentionBatchSize)
	if err != nil {
		logger.Error("retention cleanup: check results", "error", err)
	} else if deleted > 0 {
		logger.Info("retention cleanup: deleted check results", "count", deleted)
	}

	// Clean inactive endpoints
	inactiveCutoff := time.Now().Add(-inactiveEndpointRetention)
	epDeleted, err := store.DeleteInactiveEndpointsBefore(ctx, inactiveCutoff)
	if err != nil {
		logger.Error("retention cleanup: inactive endpoints", "error", err)
	} else if epDeleted > 0 {
		logger.Info("retention cleanup: deleted inactive endpoints", "count", epDeleted)
	}
}
