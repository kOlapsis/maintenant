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
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
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

	// Half the server's default per-agent allowance, leaving room for live events.
	spoolDrainRatePerSecond = 500
	spoolStatusInterval     = 10 * time.Second
)

// ErrNoStream is returned by a disabled spool with no stream attached.
var ErrNoStream = errors.New("no agent stream available")

// SpoolConfig bounds what the spool may hold; both budgets at zero disable it.
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
	SendStatus(*agentpb.SpoolStatus) error
}

// snapshotKind names the event families that restate current state. They are
// never queued: replaying a stale one would overwrite a live value.
type snapshotKind string

const (
	snapshotInventory  snapshotKind = "inventory"
	snapshotSwarm      snapshotKind = "swarm"
	snapshotKubernetes snapshotKind = "kubernetes"
	snapshotCertScan   snapshotKind = "certificate"
	snapshotHostSample snapshotKind = "host"
)

// snapshotOf classifies evt, returning false for anything worth queueing.
func snapshotOf(evt *agentpb.AgentEvent) (snapshotKind, bool) {
	switch body := evt.GetBody().(type) {
	case *agentpb.AgentEvent_Inventory:
		return snapshotInventory, true
	case *agentpb.AgentEvent_Swarm:
		return snapshotSwarm, true
	case *agentpb.AgentEvent_Kubernetes:
		return snapshotKubernetes, true
	case *agentpb.AgentEvent_Certificate:
		return snapshotCertScan, true
	case *agentpb.AgentEvent_Resource:
		// An empty container id is the host's own sample, which the server keeps
		// only as a latest value.
		if body.Resource.GetContainerId() == "" {
			return snapshotHostSample, true
		}
	}
	return "", false
}

// pendingEvent is a serialized event waiting for its batch write.
type pendingEvent struct {
	EventID    string
	ObservedAt int64
	Payload    []byte
}

// Spool is the agent's bounded outbound queue.
type Spool struct {
	cfg    SpoolConfig
	logger *slog.Logger

	mu          sync.Mutex
	store       *spoolStore
	buf         []pendingEvent
	bufBytes    int64
	sink        eventSink
	snapshots   map[snapshotKind]*agentpb.AgentEvent
	sendSeq     int64
	ackSeq      int64
	dropped     int64
	dropLogged  int64
	lastDropLog time.Time
	pausedUntil time.Time
	reported    bool
	wasDraining bool

	limiter  *rate.Limiter
	flushNow chan struct{}
	drainNow chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewSpool opens the spool for dataDir, degrading to memory only if it cannot.
func NewSpool(dataDir string, cfg SpoolConfig, logger *slog.Logger) *Spool {
	s := &Spool{
		cfg:       cfg,
		logger:    logger,
		snapshots: make(map[snapshotKind]*agentpb.AgentEvent),
		limiter:   rate.NewLimiter(spoolDrainRatePerSecond, spoolDrainRatePerSecond),
		flushNow:  make(chan struct{}, 1),
		drainNow:  make(chan struct{}, 1),
		done:      make(chan struct{}),
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

// Attach binds the spool to a live stream, rewinding to the last ack.
func (s *Spool) Attach(sink eventSink) {
	s.mu.Lock()
	s.sink = sink
	s.pausedUntil = time.Time{}
	s.reported = false
	s.mu.Unlock()
	s.Rewind()
}

// Detach unbinds the current stream. Queued events wait for the next one.
func (s *Spool) Detach() {
	s.mu.Lock()
	s.sink = nil
	s.mu.Unlock()

	if s.cfg.Enabled() {
		s.logger.Info("agent: stream lost, collecting into the spool")
	}
}

// Send queues an event; a disabled spool passes it straight to the stream.
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

	if kind, ok := snapshotOf(evt); ok {
		return s.sendSnapshot(kind, evt)
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

// sendSnapshot delivers a state snapshot and keeps the latest of its kind.
func (s *Spool) sendSnapshot(kind snapshotKind, evt *agentpb.AgentEvent) error {
	s.mu.Lock()
	s.snapshots[kind] = evt
	sink := s.sink
	s.mu.Unlock()

	if sink == nil {
		return nil
	}
	if err := sink.Send(evt); err != nil {
		s.logger.Debug("agent: snapshot not sent", "kind", string(kind), "error", err)
	}
	return nil
}

// resendSnapshots re-states current state on a fresh stream, before the backlog.
func (s *Spool) resendSnapshots() {
	s.mu.Lock()
	sink := s.sink
	pending := make([]*agentpb.AgentEvent, 0, len(s.snapshots))
	for _, evt := range s.snapshots {
		pending = append(pending, evt)
	}
	s.mu.Unlock()

	if sink == nil {
		return
	}
	for _, evt := range pending {
		if err := sink.Send(evt); err != nil {
			s.logger.Debug("agent: snapshot not resent", "error", err)
			return
		}
	}
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

// flush moves the memory buffer into the store, or trims it when there is none.
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

// enforceDiskBudget drops the oldest events once the database is past budget.
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

// recordDropped accumulates abandoned events and reports them at intervals.
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

// trimMemoryLocked drops the oldest buffered events past budget; caller holds mu.
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

// Dropped reports how many events were abandoned since the last connection.
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

// Drain sends everything queued on the attached stream, oldest first.
func (s *Spool) Drain(ctx context.Context) error {
	if !s.cfg.Enabled() {
		<-ctx.Done()
		return nil
	}

	s.resendSnapshots()
	s.reportStatus(true)

	ticker := time.NewTicker(spoolDrainInterval)
	defer ticker.Stop()
	status := time.NewTicker(spoolStatusInterval)
	defer status.Stop()

	for {
		if err := s.drainOnce(ctx); err != nil {
			return err
		}
		s.reportStatus(false)
		select {
		case <-ctx.Done():
			return nil
		case <-s.done:
			return nil
		case <-status.C:
			s.reportStatus(true)
		case <-ticker.C:
		case <-s.drainNow:
		}
	}
}

// reportStatus tells the server how far behind the agent is; without force an
// idle agent stays quiet.
func (s *Spool) reportStatus(force bool) {
	depth, err := s.Depth()
	if err != nil {
		s.logger.Debug("agent: cannot read spool depth", "error", err)
		return
	}
	dropped, err := s.Dropped()
	if err != nil {
		s.logger.Debug("agent: cannot read spool drop count", "error", err)
		return
	}
	draining := depth > 0

	s.mu.Lock()
	sink := s.sink
	changed := draining != s.wasDraining || !s.reported
	s.wasDraining = draining
	s.reported = true
	s.mu.Unlock()

	if changed {
		if draining {
			s.logger.Info("agent: replaying spooled events", "queued", depth, "dropped", dropped)
		} else {
			s.logger.Info("agent: spool drained, back in step with the server")
		}
	}
	if sink == nil || (!force && !changed && !draining) {
		return
	}
	st := &agentpb.SpoolStatus{
		Queued:              uint64(depth), // #nosec G115 -- depth is a row count, never negative
		Draining:            draining,
		DroppedSinceConnect: uint64(dropped), // #nosec G115 -- a counter, never negative
	}
	if err := sink.SendStatus(st); err != nil {
		s.logger.Debug("agent: spool status not sent", "error", err)
	}
}

func (s *Spool) drainOnce(ctx context.Context) error {
	if err := s.flush(); err != nil {
		return err
	}

	for {
		s.mu.Lock()
		sink, store, after := s.sink, s.store, s.sendSeq
		paused := time.Now().Before(s.pausedUntil)
		s.mu.Unlock()
		if sink == nil || store == nil || paused {
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
			if err := s.limiter.Wait(ctx); err != nil {
				return nil
			}
			evt.Replayed = true
			evt.Seq = uint64(row.Seq) // #nosec G115 -- an AUTOINCREMENT row id, positive by construction
			if err := sink.Send(evt); err != nil {
				return fmt.Errorf("drain spooled event: %w", err)
			}
			s.mu.Lock()
			s.sendSeq = row.Seq
			s.mu.Unlock()
		}
	}
}

// Acked records the row the server has taken everything up to.
func (s *Spool) Acked(seq uint64) {
	if seq > math.MaxInt64 {
		return
	}
	purgeTo := int64(seq)

	s.mu.Lock()
	store := s.store
	// Snapshots go out on the same stream and consume sequence numbers of their
	// own, so an ack can name a row the drain has not reached. Never purge past
	// what was actually sent from the spool.
	if purgeTo > s.sendSeq {
		purgeTo = s.sendSeq
	}
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

// Rewind moves the send cursor back to the last acknowledged event.
func (s *Spool) Rewind() {
	s.mu.Lock()
	s.sendSeq = s.ackSeq
	s.mu.Unlock()
}

// RateLimited holds the drain for the delay the server asked for and rewinds to
// the last ack: the server drops a refused event without failing the stream.
func (s *Spool) RateLimited(retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	s.mu.Lock()
	s.sendSeq = s.ackSeq
	s.pausedUntil = time.Now().Add(retryAfter)
	s.mu.Unlock()

	s.logger.Warn("agent: server refused the drain rate, backing off", "retry_after", retryAfter)
}

// Discard drops the whole spool, for a revoked agent.
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
