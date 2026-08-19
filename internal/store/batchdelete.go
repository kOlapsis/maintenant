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
	"context"
	"fmt"
	"time"
)

const defaultBatchSize = 1000

// batchOpts bounds a batched delete: batchSize caps the rows removed per
// transaction, budget caps how long the loop may hold the serialized writer.
type batchOpts struct {
	batchSize int
	budget    time.Duration // 0 = no time limit
}

func (o batchOpts) normalized() batchOpts {
	if o.batchSize <= 0 {
		o.batchSize = defaultBatchSize
	}
	return o
}

// runBatchedDelete repeats step until it stops filling a batch, the context is
// cancelled, or the time budget runs out. truncated reports that rows matching
// the cutoff are still present, so the caller should come back sooner.
//
// A cancelled context is not an error: the rows deleted so far are committed and
// the remaining ones are picked up by the next run.
func runBatchedDelete(ctx context.Context, o batchOpts, step func(context.Context, int) (int64, error)) (total int64, truncated bool, err error) {
	o = o.normalized()
	var deadline time.Time
	if o.budget > 0 {
		deadline = time.Now().Add(o.budget)
	}

	for {
		if ctx.Err() != nil {
			return total, true, nil
		}

		affected, err := step(ctx, o.batchSize)
		total += affected
		if err != nil {
			return total, true, err
		}
		// Two exit conditions rather than one: a partial batch means the table is
		// drained, and a zero batch guards against a LIMIT that yields nothing.
		if affected == 0 || affected < int64(o.batchSize) {
			return total, false, nil
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return total, true, nil
		}
	}
}

// deleteRowsBefore drains every row of table whose col is older than before.
// It deletes by rowid: the subquery is then served by the index on col alone,
// without reading the table to resolve a TEXT uuid primary key.
//
// table and col are package-level literals, never user input.
func deleteRowsBefore(ctx context.Context, w *Writer, o batchOpts, table, col string, before time.Time) (int64, bool, error) {
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE rowid IN (SELECT rowid FROM %s WHERE %s<? LIMIT ?)`,
		table, table, col)
	cutoff := before.Unix()

	total, truncated, err := runBatchedDelete(ctx, o, func(ctx context.Context, batchSize int) (int64, error) {
		res, err := w.Exec(ctx, query, cutoff, batchSize)
		if err != nil {
			return res.RowsAffected, fmt.Errorf("delete from %s: %w", table, err)
		}
		return res.RowsAffected, nil
	})
	return total, truncated, err
}
