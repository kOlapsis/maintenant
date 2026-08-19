// Copyright 2026 Benjamin Touchard (kOlapsis)
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
	"log/slog"
	"sync"
	"sync/atomic"
)

const writerBufferSize = 256

// WriteOp represents a single write operation to be serialized. When Fn is set,
// it runs inside a transaction on the writer goroutine and Query/Args are ignored.
type WriteOp struct {
	Query string
	Args  []interface{}
	Fn    func(context.Context, *sql.Tx) error
	Done  chan WriteResult
}

// WriteResult contains the result of a write operation.
type WriteResult struct {
	LastInsertID int64
	RowsAffected int64
	Err          error
}

// Writer serializes all database writes through a single goroutine.
type Writer struct {
	db      *sql.DB
	ch      chan WriteOp
	logger  *slog.Logger
	stopped atomic.Bool
	wg      sync.WaitGroup
	once    sync.Once
}

// NewWriter creates a new serialized writer.
func NewWriter(db *sql.DB, logger *slog.Logger) *Writer {
	return &Writer{
		db:     db,
		ch:     make(chan WriteOp, writerBufferSize),
		logger: logger,
	}
}

// Start begins the single-writer goroutine. Blocks until ctx is cancelled.
func (w *Writer) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Mark as stopped, wait for in-flight Exec calls, then close the channel once.
				w.once.Do(func() {
					w.stopped.Store(true)
					w.wg.Wait()
					close(w.ch)
				})
				// Drain remaining ops
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
					wr.LastInsertID, _ = result.LastInsertId()
					wr.RowsAffected, _ = result.RowsAffected()
				}
				op.Done <- wr
			}
		}
	}()
}

// Exec sends a write operation and waits for the result.
func (w *Writer) Exec(ctx context.Context, query string, args ...interface{}) (WriteResult, error) {
	return w.submit(ctx, WriteOp{Query: query, Args: args})
}

// Tx runs fn inside a single transaction executed on the writer goroutine, so
// the whole read-modify-write is serialized against every other write — no other
// write can interleave between fn's reads and its writes. fn must use the
// provided *sql.Tx for all statements; calling Exec/Tx from within fn deadlocks.
func (w *Writer) Tx(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	_, err := w.submit(ctx, WriteOp{Fn: fn})
	return err
}

// submit enqueues an op on the writer goroutine and waits for its result.
func (w *Writer) submit(ctx context.Context, op WriteOp) (WriteResult, error) {
	if w.stopped.Load() {
		return WriteResult{}, errors.New("writer is stopped")
	}
	w.wg.Add(1)
	// Re-check after Add to avoid racing with shutdown.
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

// runTx executes fn within a transaction, rolling back on any error. It runs on
// the writer goroutine, so it is the only writer for its duration.
func (w *Writer) runTx(ctx context.Context, fn func(context.Context, *sql.Tx) error) WriteResult {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return WriteResult{Err: err}
	}
	if err := fn(ctx, tx); err != nil {
		_ = tx.Rollback()
		return WriteResult{Err: err}
	}
	if err := tx.Commit(); err != nil {
		return WriteResult{Err: err}
	}
	return WriteResult{}
}
