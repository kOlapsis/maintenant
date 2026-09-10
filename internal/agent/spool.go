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

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

const (
	spoolFlushInterval = time.Second
	spoolFlushEvents   = 500
	spoolDrainInterval = 200 * time.Millisecond
	spoolDrainBatch    = 200
	spoolPurgeInterval = time.Minute
	spoolDropLogEvery  = 30 * time.Second
)

// ErrNoStream is returned by a disabled spool when no stream is attached, which
// is what a collector used to see when the connection was down.
var ErrNoStream = errors.New("no agent stream available")

// SpoolConfig bounds what the spool may hold. Both budgets at zero disable it
// and restore the pre-spool behaviour, where a failed send stops the collector.
type SpoolConfig struct {
	MaxMemoryBytes int64
	MaxDiskBytes   int64
	MaxAge         time.Duration
}

// Enabled reports whether events are queued rather than passed straight through.
func (c SpoolConfig) Enabled() bool {
	return c.MaxMemoryBytes > 0 || c.MaxDiskBytes > 0
}

// eventSink sends one event on a live stream.
type eventSink interface {
	Send(*agentpb.AgentEvent) error
}

// pendingEvent is a serialized event waiting for its batch write.
type pendingEvent struct {
	EventID    string
	ObservedAt int64
	Payload    []byte
}

// Spool is the agent's bounded outbound queue. Collectors write to it with the
// same Send signature they used on the stream, so a broken connection stops
// being their problem.
type Spool struct {
	cfg    SpoolConfig
	logger *slog.Logger

	mu          sync.Mutex
	store       *spoolStore
	buf         []pendingEvent
	bufBytes    int64
	sink        eventSink
	sendSeq     int64
	ackSeq      int64
	dropped     int64
	dropLogged  int64
	lastDropLog time.Time

	flushNow chan struct{}
	drainNow chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewSpool opens the spool for dataDir. A store that cannot be opened degrades
// to memory only rather than stopping the agent.
func NewSpool(dataDir string, cfg SpoolConfig, logger *slog.Logger) *Spool {
	s := &Spool{
		cfg:      cfg,
		logger:   logger,
		flushNow: make(chan struct{}, 1),
		drainNow: make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	if !cfg.Enabled() {
		return s
	}

	store, err := openSpoolStore(dataDir)
	if err != nil {
		logger.Warn("agent: spool falling back to memory only, queued events will not survive a restart",
			"data_dir", dataDir, "error", err)
	} else {
		s.store = store
	}
	go s.flushLoop()
	return s
}

// Attach binds the spool to a live stream and resends anything the previous one
// could not prove it delivered.
func (s *Spool) Attach(sink eventSink) {
	s.mu.Lock()
	s.sink = sink
	s.sendSeq = s.ackSeq
	s.mu.Unlock()
}

// Detach unbinds the current stream. Queued events wait for the next one.
func (s *Spool) Detach() {
	s.mu.Lock()
	s.sink = nil
	s.mu.Unlock()
}

// Send queues an event for delivery. A disabled spool passes it straight to the
// stream and reports the failure, as the collectors used to see it.
func (s *Spool) Send(evt *agentpb.AgentEvent) error {
	if !s.cfg.Enabled() {
		s.mu.Lock()
		sink := s.sink
		s.mu.Unlock()
		if sink == nil {
			return ErrNoStream
		}
		return sink.Send(evt)
	}

	payload, err := proto.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal spooled event: %w", err)
	}

	observed := time.Now().Unix()
	if ts := evt.GetObservedAt(); ts != nil {
		observed = ts.AsTime().Unix()
	}

	s.mu.Lock()
	s.buf = append(s.buf, pendingEvent{EventID: evt.GetEventId(), ObservedAt: observed, Payload: payload})
	s.bufBytes += int64(len(payload))
	overMemory := s.bufBytes >= s.cfg.MaxMemoryBytes
	full := len(s.buf) >= spoolFlushEvents
	s.mu.Unlock()

	if overMemory {
		if err := s.flush(); err != nil {
			return err
		}
	} else if full {
		s.requestFlush()
	}
	s.requestDrain()
	return nil
}

func (s *Spool) requestDrain() {
	select {
	case s.drainNow <- struct{}{}:
	default:
	}
}

func (s *Spool) requestFlush() {
	select {
	case s.flushNow <- struct{}{}:
	default:
	}
}

func (s *Spool) flushLoop() {
	flush := time.NewTicker(spoolFlushInterval)
	defer flush.Stop()
	purge := time.NewTicker(spoolPurgeInterval)
	defer purge.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-purge.C:
			if err := s.purgeExpired(); err != nil {
				s.logger.Warn("agent: spool retention purge failed", "error", err)
			}
			continue
		case <-flush.C:
		case <-s.flushNow:
		}
		if err := s.flush(); err != nil {
			s.logger.Warn("agent: spool flush failed", "error", err)
		}
	}
}

// flush moves the memory buffer into the store. With no store it enforces the
// memory budget in place, dropping the oldest events.
func (s *Spool) flush() error {
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return nil
	}
	if s.store == nil {
		dropped := s.trimMemoryLocked()
		s.mu.Unlock()
		if dropped > 0 {
			s.logger.Warn("agent: spool memory budget reached, oldest events dropped", "dropped", dropped)
		}
		return nil
	}
	batch := s.buf
	s.buf = nil
	s.bufBytes = 0
	s.mu.Unlock()

	if _, err := s.store.Append(batch); err != nil {
		s.mu.Lock()
		s.buf = append(batch, s.buf...)
		for _, ev := range batch {
			s.bufBytes += int64(len(ev.Payload))
		}
		s.mu.Unlock()
		return err
	}
	return s.enforceDiskBudget()
}

// enforceDiskBudget drops the oldest events once the database is past its
// budget, and returns the freed pages to the filesystem.
func (s *Spool) enforceDiskBudget() error {
	if s.cfg.MaxDiskBytes <= 0 {
		return nil
	}
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return nil
	}

	size, err := store.SizeBytes()
	if err != nil {
		return err
	}
	if size <= s.cfg.MaxDiskBytes {
		return nil
	}

	// Free the overshoot plus a tenth of the budget: trimming only the overshoot
	// would trigger another delete on the very next write.
	dropped, err := store.DeleteOldestBytes(size - s.cfg.MaxDiskBytes + s.cfg.MaxDiskBytes/10)
	if err != nil {
		return err
	}
	if dropped == 0 {
		return nil
	}
	if err := store.addDropped(dropped); err != nil {
		return err
	}
	s.recordDropped(dropped)
	return nil
}

// recordDropped accumulates abandoned events and reports them at intervals. A
// line per dropped event would bury the rest of the agent's log during a long
// outage.
func (s *Spool) recordDropped(n int64) {
	s.mu.Lock()
	s.dropLogged += n
	if time.Since(s.lastDropLog) < spoolDropLogEvery {
		s.mu.Unlock()
		return
	}
	total := s.dropLogged
	s.dropLogged = 0
	s.lastDropLog = time.Now()
	s.mu.Unlock()

	s.logger.Warn("agent: spool is full, oldest events dropped", "dropped", total)
}

// trimMemoryLocked drops the oldest buffered events until the memory budget is
// met. Caller holds the lock.
func (s *Spool) trimMemoryLocked() int64 {
	var dropped int64
	for s.bufBytes > s.cfg.MaxMemoryBytes && len(s.buf) > 0 {
		s.bufBytes -= int64(len(s.buf[0].Payload))
		s.buf = s.buf[1:]
		dropped++
	}
	s.dropped += dropped
	return dropped
}

// Depth reports how many events are waiting, buffered and stored.
func (s *Spool) Depth() (int64, error) {
	s.mu.Lock()
	buffered := int64(len(s.buf))
	store := s.store
	s.mu.Unlock()

	if store == nil {
		return buffered, nil
	}
	stored, err := store.Depth()
	if err != nil {
		return buffered, err
	}
	return buffered + stored, nil
}

// Dropped reports how many events were abandoned for lack of room since the
// last successful connection.
func (s *Spool) Dropped() (int64, error) {
	s.mu.Lock()
	inMemory := s.dropped
	store := s.store
	s.mu.Unlock()

	if store == nil {
		return inMemory, nil
	}
	persisted, err := store.meta(metaDroppedSinceConn)
	if err != nil {
		return inMemory, err
	}
	return inMemory + persisted, nil
}

// ResetDropped clears the per-connection drop counter.
func (s *Spool) ResetDropped() {
	s.mu.Lock()
	s.dropped = 0
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return
	}
	if err := store.resetDroppedSinceConnect(); err != nil {
		s.logger.Warn("agent: cannot reset spool drop counter", "error", err)
	}
}

// PurgeExpired drops everything observed before the retention window.
func (s *Spool) PurgeExpired(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	return s.purgeExpired()
}

func (s *Spool) purgeExpired() error {
	if s.cfg.MaxAge <= 0 {
		return nil
	}
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return nil
	}

	n, err := store.DeleteOlderThan(time.Now().Add(-s.cfg.MaxAge).Unix())
	if err != nil {
		return err
	}
	if n > 0 {
		s.logger.Info("agent: spool dropped events past the retention window", "count", n)
	}
	return nil
}

// Close flushes what is buffered and releases the store.
func (s *Spool) Close() error {
	var err error
	s.stopOnce.Do(func() {
		close(s.done)
		err = s.flush()
		s.mu.Lock()
		store := s.store
		s.store = nil
		s.mu.Unlock()
		if store != nil {
			if cerr := store.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
	})
	return err
}

// Drain sends everything queued on the attached stream, oldest first, and keeps
// sending what the collectors produce, until the stream breaks or ctx ends.
func (s *Spool) Drain(ctx context.Context) error {
	if !s.cfg.Enabled() {
		<-ctx.Done()
		return nil
	}

	ticker := time.NewTicker(spoolDrainInterval)
	defer ticker.Stop()

	for {
		if err := s.drainOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.done:
			return nil
		case <-ticker.C:
		case <-s.drainNow:
		}
	}
}

func (s *Spool) drainOnce(ctx context.Context) error {
	if err := s.flush(); err != nil {
		return err
	}

	for {
		s.mu.Lock()
		sink, store, after := s.sink, s.store, s.sendSeq
		s.mu.Unlock()
		if sink == nil || store == nil {
			return nil
		}

		batch, err := store.ReadFrom(after, spoolDrainBatch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}

		for _, row := range batch {
			if err := ctx.Err(); err != nil {
				return nil
			}
			evt := &agentpb.AgentEvent{}
			if err := proto.Unmarshal(row.Payload, evt); err != nil {
				s.logger.Warn("agent: dropping unreadable spooled event", "seq", row.Seq, "error", err)
				s.mu.Lock()
				s.sendSeq = row.Seq
				s.mu.Unlock()
				continue
			}
			evt.Replayed = true
			evt.Seq = uint64(row.Seq)
			if err := sink.Send(evt); err != nil {
				return fmt.Errorf("drain spooled event: %w", err)
			}
			s.mu.Lock()
			s.sendSeq = row.Seq
			s.mu.Unlock()
		}
	}
}

// Acked records the row the server has taken everything up to, which is what
// finally lets those rows go. A Send that returned nil only proves the event
// reached the transport buffer.
func (s *Spool) Acked(seq uint64) {
	purgeTo := int64(seq)

	s.mu.Lock()
	store := s.store
	// An event sent outside the spool carries a lower number, and its ack must
	// not walk the purge cursor backwards.
	if purgeTo <= s.ackSeq {
		s.mu.Unlock()
		return
	}
	s.ackSeq = purgeTo
	s.mu.Unlock()

	if store == nil {
		return
	}
	if _, err := store.DeleteUpTo(purgeTo); err != nil {
		s.logger.Warn("agent: cannot purge acknowledged spool events", "error", err)
	}
}

// Rewind moves the send cursor back to the last acknowledged event, so a stream
// that broke mid-drain resends what it cannot prove arrived.
func (s *Spool) Rewind() {
	s.mu.Lock()
	s.sendSeq = s.ackSeq
	s.mu.Unlock()
}

// Discard drops the whole spool. Used when the server revokes the agent.
func (s *Spool) Discard() error {
	s.mu.Lock()
	store := s.store
	s.store = nil
	s.buf = nil
	s.bufBytes = 0
	s.mu.Unlock()

	if store == nil {
		return nil
	}
	path := store.path
	if err := store.Close(); err != nil {
		return err
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove spool file %s: %w", p, err)
		}
	}
	return nil
}
