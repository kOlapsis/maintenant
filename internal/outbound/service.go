// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/ssrf"
	"github.com/kolapsis/maintenant/internal/uid"
)

const (
	tickInterval = 5 * time.Second
	sendTimeout  = 10 * time.Second
)

// Deps holds the service dependencies.
type Deps struct {
	Store       Store
	Logger      *slog.Logger
	Version     string
	Client      *http.Client
	ValidateURL func(ctx context.Context, rawURL string) error
}

// Service manages outbound heartbeats and sends them when due.
type Service struct {
	store     Store
	logger    *slog.Logger
	userAgent string
	client    *http.Client
	validate  func(ctx context.Context, rawURL string) error
	now       func() time.Time

	mu       sync.Mutex
	inFlight map[string]struct{}
	wg       sync.WaitGroup
}

// NewService creates an outbound heartbeat service.
func NewService(d Deps) *Service {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	client := d.Client
	if client == nil {
		client = ssrf.NewHTTPClient(sendTimeout, false)
	}
	validate := d.ValidateURL
	if validate == nil {
		validate = ssrf.ValidateURL
	}
	return &Service{
		store:     d.Store,
		logger:    logger,
		userAgent: "maintenant/" + d.Version + " outbound-heartbeat",
		client:    client,
		validate:  validate,
		now:       time.Now,
		inFlight:  make(map[string]struct{}),
	}
}

// List returns every outbound heartbeat.
func (s *Service) List(ctx context.Context) ([]*OutboundHeartbeat, error) {
	list, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*OutboundHeartbeat{}
	}
	return list, nil
}

// Get returns one outbound heartbeat or ErrNotFound.
func (s *Service) Get(ctx context.Context, id string) (*OutboundHeartbeat, error) {
	o, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrNotFound
	}
	return o, nil
}

func (s *Service) validateInput(ctx context.Context, in *Input) error {
	if err := in.Validate(); err != nil {
		return err
	}
	if err := s.validate(ctx, in.URL); err != nil {
		return fmt.Errorf("%w: url: %v", ErrInvalidInput, err)
	}
	return nil
}

// Create validates and stores a new outbound heartbeat.
func (s *Service) Create(ctx context.Context, in Input) (*OutboundHeartbeat, error) {
	if err := s.validateInput(ctx, &in); err != nil {
		return nil, err
	}
	now := s.now().Truncate(time.Second)
	o := &OutboundHeartbeat{
		ID:              uid.New(),
		Name:            in.Name,
		URL:             in.URL,
		IntervalSeconds: in.IntervalSeconds,
		Enabled:         in.enabled(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.store.Create(ctx, o); err != nil {
		return nil, fmt.Errorf("create outbound heartbeat: %w", err)
	}
	return o, nil
}

// Update replaces the editable fields of an outbound heartbeat.
func (s *Service) Update(ctx context.Context, id string, in Input) (*OutboundHeartbeat, error) {
	if err := s.validateInput(ctx, &in); err != nil {
		return nil, err
	}
	o, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	o.Name = in.Name
	o.URL = in.URL
	o.IntervalSeconds = in.IntervalSeconds
	o.Enabled = in.enabled()
	o.UpdatedAt = s.now().Truncate(time.Second)
	found, err := s.store.Update(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("update outbound heartbeat: %w", err)
	}
	if !found {
		return nil, ErrNotFound
	}
	return o, nil
}

// Delete removes an outbound heartbeat.
func (s *Service) Delete(ctx context.Context, id string) error {
	found, err := s.store.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("delete outbound heartbeat: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

// SendNow sends one outbound heartbeat immediately and returns it updated.
func (s *Service) SendNow(ctx context.Context, id string) (*OutboundHeartbeat, error) {
	o, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.claim(o.ID) {
		defer s.release(o.ID)
	}
	if err := s.send(ctx, o); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

// Start runs the send loop until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	go func() {
		s.logger.Info("outbound heartbeat: sender started")
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.wg.Wait()
				return
			case <-ticker.C:
				s.sendDue(ctx)
			}
		}
	}()
}

func (s *Service) sendDue(ctx context.Context) {
	list, err := s.store.List(ctx)
	if err != nil {
		s.logger.Error("outbound heartbeat: list targets", "error", err)
		return
	}
	now := s.now()
	for _, o := range list {
		if !o.due(now) || !s.claim(o.ID) {
			continue
		}
		s.wg.Add(1)
		go func(o *OutboundHeartbeat) {
			defer s.wg.Done()
			defer s.release(o.ID)
			if err := s.send(ctx, o); err != nil && ctx.Err() == nil {
				s.logger.Error("outbound heartbeat: record result", "id", o.ID, "error", err)
			}
		}(o)
	}
}

func (s *Service) claim(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.inFlight[id]; busy {
		return false
	}
	s.inFlight[id] = struct{}{}
	return true
}

func (s *Service) release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inFlight, id)
}

func (s *Service) send(ctx context.Context, o *OutboundHeartbeat) error {
	res := SendResult{SentAt: s.now()}
	code, err := s.get(ctx, o.URL)
	switch {
	case err != nil:
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		msg := err.Error()
		res.Error = &msg
	case code < 200 || code > 299:
		msg := fmt.Sprintf("unexpected status %d %s", code, http.StatusText(code))
		res.StatusCode = &code
		res.Error = &msg
	default:
		res.StatusCode = &code
	}
	if res.Error != nil && ctx.Err() == nil {
		s.logger.Warn("outbound heartbeat: send failed",
			"id", o.ID, "name", o.Name, "error", *res.Error)
	}
	return s.store.RecordSend(ctx, o.ID, res)
}

func (s *Service) get(ctx context.Context, target string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, nil
}
