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

package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	shm "github.com/kolapsis/shm/sdk/golang"
)

// Defaults for the wire-relevant constants. Per spec FR-008 these are not
// operator-configurable; the cfg struct exists so tests can override them.
const (
	defaultEndpoint    = "https://metrics.kolapsis.com"
	defaultDataDir     = "/data/shm"
	defaultEnvironment = "production"
	appName            = "Maintenant"
)

// Disable reasons used in the single startup log line (FR-017).
const (
	reasonOptOut            = "opt-out"
	reasonDataDirUnwritable = "datadir-unwritable"
)

// Config carries the wire-relevant constants for the telemetry subsystem.
// All fields have safe defaults populated by New when unset.
type Config struct {
	Disabled    bool   // true if MAINTENANT_DISABLE_TELEMETRY is truthy
	Endpoint    string // default: defaultEndpoint
	DataDir     string // default: defaultDataDir
	AppVersion  string // injected via ldflags at build time
	Environment string // default: defaultEnvironment
}

// Deps groups the count sources and edition source needed by the metrics
// provider. Each store satisfies the `counter` interface structurally.
type Deps struct {
	Containers       counter
	Endpoints        counter
	Heartbeats       counter
	Certificates     counter
	Webhooks         counter
	StatusComponents counter
	Edition          editionSource
}

// Service owns the SHM SDK client and its single reporting goroutine.
// Disabled instances (via opt-out or unwritable datadir) hold a nil client
// and treat Start as a no-op.
type Service struct {
	cfg    Config
	logger *slog.Logger
	client *shm.Client
}

// New constructs the telemetry service. It evaluates the opt-out and
// datadir-writability checks up-front and emits exactly one startup log
// line describing the resolved state (FR-017).
//
// New never returns an error: failures during initialization disable
// telemetry for the lifetime of the process and are reported via the log
// line. This matches the spec's "best-effort, never destabilize the host"
// posture (FR-009 / FR-010 / FR-016).
func New(cfg Config, deps Deps, logger *slog.Logger) *Service {
	cfg = applyDefaults(cfg)
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Disabled {
		logger.Info("telemetry disabled", "reason", reasonOptOut)
		return &Service{cfg: cfg, logger: logger}
	}

	if err := ensureDataDirWritable(cfg.DataDir); err != nil {
		logger.Warn("telemetry disabled",
			"reason", reasonDataDirUnwritable,
			"datadir", cfg.DataDir,
			"error", err,
		)
		return &Service{cfg: cfg, logger: logger}
	}

	provider := newMetricsProvider(deps, logger)
	client, err := shm.New(shm.Config{
		ServerURL:            cfg.Endpoint,
		AppName:              appName,
		AppVersion:           cfg.AppVersion,
		DataDir:              cfg.DataDir,
		Environment:          cfg.Environment,
		Enabled:              true,
		CollectSystemMetrics: true,
	})
	if err != nil {
		logger.Warn("telemetry disabled",
			"reason", "sdk-init-failed",
			"error", fmt.Errorf("telemetry: %w", err),
		)
		return &Service{cfg: cfg, logger: logger}
	}
	client.SetProvider(shm.MetricsProvider(provider))

	resolvedEdition := safeEdition(deps.Edition, logger)
	logger.Info("telemetry enabled",
		"endpoint", cfg.Endpoint,
		"edition", resolvedEdition,
		"datadir", cfg.DataDir,
	)

	return &Service{cfg: cfg, logger: logger, client: client}
}

// Start spawns the SDK reporting goroutine if telemetry is enabled.
// The goroutine self-exits when ctx is cancelled. A panic inside the SDK
// is contained and logged (FR-012); it never propagates to the host.
//
// Start is a no-op when telemetry is disabled. Calling Start more than
// once is allowed but only the first invocation spawns a goroutine.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.client == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Warn("telemetry panic recovered",
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		s.client.Start(ctx)
	}()
}

// IsActive reports whether the SDK client is initialised. Used by tests to
// assert opt-out and datadir-unwritable branches did not spawn the goroutine.
func (s *Service) IsActive() bool {
	return s != nil && s.client != nil
}

func applyDefaults(cfg Config) Config {
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaultEndpoint
	}
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDataDir
	}
	if cfg.Environment == "" {
		cfg.Environment = defaultEnvironment
	}
	return cfg
}
