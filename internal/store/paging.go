// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
)

const maxPageSize = 500

// pageQuery is one page of a list: countSQL and selectSQL take the same args, selectSQL has its ORDER BY but no LIMIT.
type pageQuery struct {
	what         string
	countSQL     string
	selectSQL    string
	args         []any
	limit        int
	offset       int
	defaultLimit int
}

// listPage returns the rows of one page and the total number of rows matching q.
func listPage[T any](ctx context.Context, db *Reader, q pageQuery, scan func(rowScanner) (T, error)) ([]T, int, error) {
	var total int
	if err := db.QueryRowContext(ctx, q.countSQL, q.args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count %s: %w", q.what, err)
	}

	limit := q.limit
	if limit <= 0 {
		limit = q.defaultLimit
	}
	limit = min(limit, maxPageSize)
	query := q.selectSQL + fmt.Sprintf(` LIMIT %d`, limit)
	if q.offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, q.offset) // #nosec G202 -- integer formatting of an int option.
	}

	rows, err := db.QueryContext(ctx, query, q.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list %s: %w", q.what, err)
	}
	defer func() { _ = rows.Close() }()

	var results []T
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, rows.Err()
}
