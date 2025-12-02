package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
)

const writerBufferSize = 256

// WriteOp represents a single write operation to be serialized.
type WriteOp struct {
	Query string
	Args  []interface{}
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

	op := WriteOp{
		Query: query,
		Args:  args,
		Done:  make(chan WriteResult, 1),
	}
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
