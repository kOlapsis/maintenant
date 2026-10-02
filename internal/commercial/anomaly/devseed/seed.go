// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package devseed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolapsis/maintenant/internal/commercial/anomaly"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/uid"
)

// minObservedHours is the real history, two day/night cycles, a container needs before it is worth extending.
const minObservedHours = 48

const seedTxBatch = 1000

// Options configures one seeding run.
type Options struct {
	DBPath string
	Span   string // "28d" or a Go duration; empty means reset only
	Reset  bool
}

// Report summarises what a run did.
type Report struct {
	Seeded        int
	Skipped       int
	RowsWritten   int
	RowsWithdrawn int
}

// Run opens and migrates the SQLite database at opts.DBPath, seeds it, then closes it.
func Run(ctx context.Context, opts Options, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	var span time.Duration
	switch {
	case opts.Span != "":
		var err error
		if span, err = ParseSpan(opts.Span); err != nil {
			return err
		}
	case !opts.Reset:
		return fmt.Errorf("nothing to do: pass a span, --seed-anomaly-reset, or both")
	}

	db, err := store.Open(opts.DBPath, logger)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := store.Migrate(ctx, db, logger); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	writerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	db.StartWriter(writerCtx)

	rep, err := seed(writerCtx, db, manifestPath(opts.DBPath), span, opts.Reset, logger)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "anomaly: seed complete",
		"containers", rep.Seeded, "skipped", rep.Skipped, "rows", rep.RowsWritten,
		"withdrawn", rep.RowsWithdrawn, "span", span.String())
	return nil
}

// Seed fills the hours missing from the last span of every container with enough real history, after clearing the learned state when reset is set.
func Seed(ctx context.Context, db *store.DB, dbPath string, span time.Duration, reset bool, logger *slog.Logger) (Report, error) {
	return seed(ctx, db, manifestPath(dbPath), span, reset, logger)
}

func seed(ctx context.Context, db *store.DB, manifestFile string, span time.Duration, reset bool, logger *slog.Logger) (Report, error) {
	if logger == nil {
		logger = slog.Default()
	}
	var rep Report

	if reset {
		withdrawn, err := withdrawSeeded(ctx, db, manifestFile)
		if err != nil {
			return rep, err
		}
		if err := clearLearnedState(ctx, db); err != nil {
			return rep, err
		}
		rep.RowsWithdrawn = withdrawn
		logger.InfoContext(ctx, "anomaly: learned state cleared", "seeded_hours_withdrawn", withdrawn)
	}
	if span <= 0 {
		return rep, nil
	}
	seeded := loadManifest(manifestFile)

	cfg := anomaly.ConfigFromEnv(logger)
	if want := time.Duration(cfg.BaselineWindowDays) * 24 * time.Hour; span < want {
		logger.WarnContext(ctx, "anomaly: seed span is shorter than the baseline window, series cannot reach ready",
			"span", span.String(), "baseline_window", want.String())
	}

	// The range runs through the hour in progress: the engine reads [now-window, now], and the real rollup overwrites that hour once it closes.
	currentHour := time.Now().Truncate(time.Hour)
	to := currentHour.Add(time.Hour).Unix()
	from := currentHour.Add(-span).Unix()

	targets, skipped, err := selectContainers(ctx, db, from)
	if err != nil {
		return rep, err
	}
	rep.Skipped = skipped

	for _, id := range targets {
		rows, err := loadHourly(ctx, db, id, from)
		if err != nil {
			return rep, err
		}
		present := make(map[int64]struct{}, len(rows))
		for _, r := range rows {
			present[r.Bucket] = struct{}{}
		}
		missing := MissingBuckets(from, to, present)
		rep.Seeded++
		if len(missing) == 0 {
			continue
		}

		cp := BuildProfile(rows, cfg.Location)
		written, err := writeRows(ctx, db, id, cp, missing, cfg.Location)
		if err != nil {
			return rep, err
		}
		seeded.add(id, missing[:written])
		rep.RowsWritten += written
		logger.InfoContext(ctx, "anomaly: container history extended",
			"container_id", id, "observed_hours", len(rows), "written_hours", written)
	}

	if rep.RowsWritten > 0 {
		if err := seeded.save(manifestFile); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// withdrawSeeded deletes the hours the seeder recorded writing, even those the real rollup has since rewritten.
func withdrawSeeded(ctx context.Context, db *store.DB, manifestFile string) (int, error) {
	seeded := loadManifest(manifestFile)
	total := 0
	for containerID, buckets := range seeded.Buckets {
		for start := 0; start < len(buckets); start += seedTxBatch {
			batch := buckets[start:min(start+seedTxBatch, len(buckets))]
			err := db.Writer().Tx(ctx, func(ctx context.Context, tx *store.Tx) error {
				for _, bucket := range batch {
					res, err := tx.ExecContext(ctx,
						`DELETE FROM resource_hourly WHERE container_id = ? AND bucket = ?`, containerID, bucket)
					if err != nil {
						return fmt.Errorf("withdraw seeded hour %d: %w", bucket, err)
					}
					n, _ := res.RowsAffected()
					total += int(n)
				}
				return nil
			})
			if err != nil {
				return total, err
			}
		}
	}
	return total, removeManifest(manifestFile)
}

// selectContainers returns the containers worth seeding and how many were left out.
func selectContainers(ctx context.Context, db *store.DB, from int64) (ids []string, skipped int, err error) {
	rows, err := db.Reader().QueryContext(ctx,
		`SELECT c.id, COUNT(rh.bucket)
		FROM containers c
		LEFT JOIN resource_hourly rh ON rh.container_id = c.id AND rh.bucket >= ?
		WHERE c.archived = 0 AND c.is_ignored = 0
		GROUP BY c.id`, from)
	if err != nil {
		return nil, 0, fmt.Errorf("select seed targets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id string
		var observed int
		if err := rows.Scan(&id, &observed); err != nil {
			return nil, 0, fmt.Errorf("scan seed target: %w", err)
		}
		if observed < minObservedHours {
			skipped++
			continue
		}
		ids = append(ids, id)
	}
	return ids, skipped, rows.Err()
}

func loadHourly(ctx context.Context, db *store.DB, containerID string, from int64) ([]HourlyRow, error) {
	rows, err := db.Reader().QueryContext(ctx,
		`SELECT bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit, avg_net_rx_bytes, avg_net_tx_bytes, sample_count
		FROM resource_hourly WHERE container_id = ? AND bucket >= ? ORDER BY bucket`, containerID, from)
	if err != nil {
		return nil, fmt.Errorf("load hourly for %s: %w", containerID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []HourlyRow
	for rows.Next() {
		var r HourlyRow
		if err := rows.Scan(&r.Bucket, &r.CPUPercent, &r.MemUsed, &r.MemLimit,
			&r.NetRxBytes, &r.NetTxBytes, &r.SampleCount); err != nil {
			return nil, fmt.Errorf("scan hourly for %s: %w", containerID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// writeRows inserts the generated missing hours; every bucket is known absent, so no conflict clause is needed.
func writeRows(ctx context.Context, db *store.DB, containerID string, cp ContainerProfile, missing []int64, loc *time.Location) (int, error) {
	written := 0
	for start := 0; start < len(missing); start += seedTxBatch {
		batch := missing[start:min(start+seedTxBatch, len(missing))]
		err := db.Writer().Tx(ctx, func(ctx context.Context, tx *store.Tx) error {
			for _, bucket := range batch {
				r := cp.Generate(containerID, bucket, loc)
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO resource_hourly
					(id, container_id, bucket, avg_cpu_percent, avg_mem_used, avg_mem_limit,
					 avg_net_rx_bytes, avg_net_tx_bytes, sample_count)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					uid.New(), containerID, r.Bucket, r.CPUPercent, int64(r.MemUsed), int64(r.MemLimit),
					int64(r.NetRxBytes), int64(r.NetTxBytes), r.SampleCount); err != nil {
					return fmt.Errorf("insert seeded hour %d: %w", bucket, err)
				}
			}
			return nil
		})
		if err != nil {
			return written, err
		}
		written += len(batch)
	}
	return written, nil
}

// clearLearnedState wipes what the detection engine derived, so learning can be replayed.
func clearLearnedState(ctx context.Context, db *store.DB) error {
	return db.Writer().Tx(ctx, func(ctx context.Context, tx *store.Tx) error {
		for _, table := range []string{"anomaly_event", "anomaly_baseline", "anomaly_series_state"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil { // #nosec G202 -- fixed table list, no input
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		return nil
	})
}
