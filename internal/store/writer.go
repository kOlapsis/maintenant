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
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

const writerBufferSize = 256

type WriteOp struct {
	Query string
	Args  []interface{}
	Fn    func(context.Context, *Tx) error
	Done  chan WriteResult
}

type WriteResult struct {
	RowsAffected int64
	Err          error
}

// Tx wraps *sql.Tx so statements written with `?` placeholders run on either
// engine. It is what Writer.Tx callbacks receive.
type Tx struct {
	tx      *sql.Tx
	dialect Dialect
}

func (t *Tx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.tx.ExecContext(ctx, t.dialect.Rebind(query), args...)
}

func (t *Tx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, t.dialect.Rebind(query), args...)
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRowContext(ctx, t.dialect.Rebind(query), args...)
}

// Serialize makes check-then-write sections of this transaction mutually
// exclusive across writers of table. On SQLite the single writer goroutine
// already guarantees it, so this is free; on PostgreSQL concurrent
// transactions would otherwise all pass the check before any of them writes.
// SHARE ROW EXCLUSIVE conflicts with itself without blocking readers.
func (t *Tx) Serialize(ctx context.Context, table string) error {
	if t.dialect != DialectPostgres {
		return nil
	}
	// table is a package-level literal, never user input.
	if _, err := t.tx.ExecContext(ctx, "LOCK TABLE "+table+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("serialize writers of %s: %w", table, err)
	}
	return nil
}

// Writer serializes writes through a single goroutine on SQLite, working
// around its single-writer discipline. On PostgreSQL the engine governs
// concurrency itself, so writes go straight to the pool: funneling them
// through one goroutine would cap the engine at a single writer for nothing.
type Writer struct {
	db      *sql.DB
	dialect Dialect
	ch      chan WriteOp
	logger  *slog.Logger
	stopped atomic.Bool
	wg      sync.WaitGroup
	once    sync.Once
}

func NewWriter(db *sql.DB, dialect Dialect, logger *slog.Logger) *Writer {
	return &Writer{
		db:      db,
		dialect: dialect,
		ch:      make(chan WriteOp, writerBufferSize),
		logger:  logger,
	}
}

func (w *Writer) Start(ctx context.Context) {
	if w.dialect == DialectPostgres {
		// Direct path: nothing to run in the background.
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				w.once.Do(func() {
					w.stopped.Store(true)
					w.wg.Wait()
					close(w.ch)
				})
				for op := range w.ch {
					op.Done <- WriteResult{Err: ctx.Err()}
				}
				return
			case op, ok := <-w.ch:
				if !ok {
					return
				}
				if op.Fn != nil {
					op.Done <- w.runTx(ctx, op.Fn)
					continue
				}
				result, err := w.db.ExecContext(ctx, op.Query, op.Args...)
				wr := WriteResult{Err: err}
				if err == nil {
					wr.RowsAffected, _ = result.RowsAffected()
				}
				op.Done <- wr
			}
		}
	}()
}

func (w *Writer) Exec(ctx context.Context, query string, args ...interface{}) (WriteResult, error) {
	query = w.dialect.Rebind(query)
	if w.dialect == DialectPostgres {
		result, err := w.db.ExecContext(ctx, query, args...)
		wr := WriteResult{Err: err}
		if err == nil {
			wr.RowsAffected, _ = result.RowsAffected()
		}
		return wr, err
	}
	return w.submit(ctx, WriteOp{Query: query, Args: args})
}

func (w *Writer) Tx(ctx context.Context, fn func(context.Context, *Tx) error) error {
	if w.dialect == DialectPostgres {
		res := w.runTx(ctx, fn)
		return res.Err
	}
	_, err := w.submit(ctx, WriteOp{Fn: fn})
	return err
}

func (w *Writer) submit(ctx context.Context, op WriteOp) (WriteResult, error) {
	if w.stopped.Load() {
		return WriteResult{}, errors.New("writer is stopped")
	}
	w.wg.Add(1)
	if w.stopped.Load() {
		w.wg.Done()
		return WriteResult{}, errors.New("writer is stopped")
	}
	defer w.wg.Done()

	op.Done = make(chan WriteResult, 1)
	select {
	case w.ch <- op:
	case <-ctx.Done():
		return WriteResult{}, ctx.Err()
	}
	select {
	case res := <-op.Done:
		return res, res.Err
	case <-ctx.Done():
		return WriteResult{}, ctx.Err()
	}
}

func (w *Writer) runTx(ctx context.Context, fn func(context.Context, *Tx) error) WriteResult {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return WriteResult{Err: err}
	}
	if err := fn(ctx, &Tx{tx: tx, dialect: w.dialect}); err != nil {
		_ = tx.Rollback()
		return WriteResult{Err: err}
	}
	if err := tx.Commit(); err != nil {
		return WriteResult{Err: err}
	}
	return WriteResult{}
}
