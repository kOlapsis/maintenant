// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package outbound periodically pings admin-configured URLs so that another
// monitoring system notices when this instance stops.
package outbound

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	MinIntervalSeconds = 30
	MaxIntervalSeconds = 86400
	MaxNameLength      = 255
)

var (
	ErrNotFound     = errors.New("outbound heartbeat not found")
	ErrInvalidInput = errors.New("invalid input")
)

// OutboundHeartbeat is a URL this instance sends a GET to at a fixed interval.
type OutboundHeartbeat struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	IntervalSeconds int        `json:"interval_seconds"`
	Enabled         bool       `json:"enabled"`
	LastSentAt      *time.Time `json:"last_sent_at"`
	LastStatusCode  *int       `json:"last_status_code"`
	LastError       *string    `json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Input is the admin-editable part of an outbound heartbeat.
type Input struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	Enabled         *bool  `json:"enabled"`
}

// SendResult is the outcome of one send.
type SendResult struct {
	SentAt     time.Time
	StatusCode *int
	Error      *string
}

// Store persists outbound heartbeats; Get returns nil, nil when the id is unknown.
type Store interface {
	List(ctx context.Context) ([]*OutboundHeartbeat, error)
	Get(ctx context.Context, id string) (*OutboundHeartbeat, error)
	Create(ctx context.Context, o *OutboundHeartbeat) error
	Update(ctx context.Context, o *OutboundHeartbeat) (bool, error)
	Delete(ctx context.Context, id string) (bool, error)
	RecordSend(ctx context.Context, id string, r SendResult) error
}

// Validate normalises the input and reports the first invalid field.
func (in *Input) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(in.Name) > MaxNameLength {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalidInput, MaxNameLength)
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: url must be an absolute https URL", ErrInvalidInput)
	}
	if in.IntervalSeconds < MinIntervalSeconds || in.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("%w: interval_seconds must be between %d and %d",
			ErrInvalidInput, MinIntervalSeconds, MaxIntervalSeconds)
	}
	return nil
}

func (in Input) enabled() bool {
	return in.Enabled == nil || *in.Enabled
}

func (o *OutboundHeartbeat) due(now time.Time) bool {
	if !o.Enabled {
		return false
	}
	if o.LastSentAt == nil {
		return true
	}
	return !o.LastSentAt.Add(time.Duration(o.IntervalSeconds) * time.Second).After(now)
}
