package endpoint

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// CheckResultCallback is called when a check completes.
type CheckResultCallback func(endpointID int64, result CheckResult)

// endpointRunner tracks a running per-endpoint goroutine.
type endpointRunner struct {
	cancel context.CancelFunc
	ep     *Endpoint
}

// CheckEngine manages per-endpoint check goroutines.
type CheckEngine struct {
	runners  sync.Map // map[int64]*endpointRunner
	callback CheckResultCallback
	logger   *slog.Logger
	wg       sync.WaitGroup
}

// NewCheckEngine creates a new check engine.
func NewCheckEngine(callback CheckResultCallback, logger *slog.Logger) *CheckEngine {
	return &CheckEngine{
		callback: callback,
		logger:   logger,
	}
}

// AddEndpoint starts a check goroutine for the given endpoint.
// If a goroutine already exists for this endpoint, it is stopped first.
func (e *CheckEngine) AddEndpoint(ctx context.Context, ep *Endpoint) {
	e.RemoveEndpoint(ep.ID)

	runCtx, cancel := context.WithCancel(ctx)
	runner := &endpointRunner{
		cancel: cancel,
		ep:     ep,
	}
	e.runners.Store(ep.ID, runner)

	e.wg.Add(1)
	go e.runLoop(runCtx, ep, &e.wg)

	e.logger.Info("started endpoint check",
		"endpoint_id", ep.ID,
		"type", ep.EndpointType,
		"target", ep.Target,
		"interval", ep.Config.Interval,
	)
}

// RemoveEndpoint stops the check goroutine for the given endpoint ID.
func (e *CheckEngine) RemoveEndpoint(endpointID int64) {
	if val, loaded := e.runners.LoadAndDelete(endpointID); loaded {
		runner := val.(*endpointRunner)
		runner.cancel()
		ClearLinkLocalWarning(endpointID)
		e.logger.Info("stopped endpoint check", "endpoint_id", endpointID)
	}
}

// ReconfigureEndpoint stops and restarts the check goroutine with updated configuration.
func (e *CheckEngine) ReconfigureEndpoint(ctx context.Context, ep *Endpoint) {
	e.AddEndpoint(ctx, ep)
}

// Stop cancels all running check goroutines and waits for them to finish.
func (e *CheckEngine) Stop() {
	e.runners.Range(func(key, value any) bool {
		runner := value.(*endpointRunner)
		runner.cancel()
		e.runners.Delete(key)
		return true
	})
	e.wg.Wait()
}

// ActiveCount returns the number of active check goroutines.
func (e *CheckEngine) ActiveCount() int {
	count := 0
	e.runners.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (e *CheckEngine) runLoop(ctx context.Context, ep *Endpoint, wg *sync.WaitGroup) {
	defer wg.Done()

	interval := ep.Config.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	// Run an initial check immediately
	e.executeCheck(ctx, ep)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.executeCheck(ctx, ep)
		}
	}
}

func (e *CheckEngine) executeCheck(ctx context.Context, ep *Endpoint) {
	var result CheckResult

	switch ep.EndpointType {
	case TypeHTTP:
		result = CheckHTTP(ctx, ep, e.logger)
	case TypeTCP:
		result = CheckTCP(ctx, ep, e.logger)
	default:
		e.logger.Warn("unknown endpoint type", "endpoint_id", ep.ID, "type", ep.EndpointType)
		return
	}

	if e.callback != nil {
		e.callback(ep.ID, result)
	}
}
