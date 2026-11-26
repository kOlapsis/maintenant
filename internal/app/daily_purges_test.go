// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestStartDailyPurges_RunsAtStart(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	updateStore := store.NewUpdateStore(db)

	old := time.Now().Add(-100 * 24 * time.Hour)
	_, err := db.Writer().Exec(context.Background(),
		`INSERT INTO alerts (id, source, alert_type, severity, status, message,
		 entity_type, entity_id, entity_name, fired_at, resolved_at, created_at)
		 VALUES ('old-alert','container','unhealthy','critical','resolved','down','container','c1','web',?,?,?)`,
		old.Unix(), old.Unix(), old.Unix())
	require.NoError(t, err)
	_, err = updateStore.InsertScanRecord(context.Background(), &update.ScanRecord{StartedAt: old, Status: update.ScanStatusCompleted})
	require.NoError(t, err)

	a := &App{alertStore: store.NewAlertStore(db), updateStore: updateStore, logger: logger}
	ctx, cancel := context.WithCancel(context.Background())
	a.startDailyPurges(ctx)
	t.Cleanup(func() {
		cancel()
		a.background.Wait()
	})

	count := func(table string) int {
		var n int
		if err := db.Reader().QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			return -1
		}
		return n
	}
	require.Eventually(t, func() bool { return count("alerts") == 0 }, 10*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return count("image_update_scans") == 0 }, 10*time.Second, 10*time.Millisecond)
}
