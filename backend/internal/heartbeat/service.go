package heartbeat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// EventCallback is called when a heartbeat event occurs (for SSE broadcasting).
type EventCallback func(eventType string, data interface{})

// AlertCallback is called when an alert or recovery event occurs.
type AlertCallback func(h *Heartbeat, alertType string, details map[string]interface{})

// LicenseChecker determines license-gated capabilities.
type LicenseChecker interface {
	CanCreateHeartbeat(currentCount int) bool
	CanStorePayload() bool
}

// DefaultLicenseChecker implements Community edition limits.
type DefaultLicenseChecker struct {
	MaxHeartbeats int
}

func (c *DefaultLicenseChecker) CanCreateHeartbeat(currentCount int) bool {
	return currentCount < c.MaxHeartbeats
}

func (c *DefaultLicenseChecker) CanStorePayload() bool {
	return false
}

var (
	ErrHeartbeatNotFound  = errors.New("heartbeat not found")
	ErrLimitReached       = errors.New("heartbeat limit reached")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidExitCode    = errors.New("invalid exit code")
)

// Service orchestrates heartbeat monitoring logic.
type Service struct {
	store          HeartbeatStore
	logger         *slog.Logger
	onEvent        EventCallback
	alertCallback  AlertCallback
	licenseChecker LicenseChecker
	baseURL        string
}

// NewService creates a new heartbeat service.
func NewService(store HeartbeatStore, logger *slog.Logger, licenseChecker LicenseChecker) *Service {
	if licenseChecker == nil {
		licenseChecker = &DefaultLicenseChecker{MaxHeartbeats: 10}
	}
	return &Service{
		store:          store,
		logger:         logger,
		licenseChecker: licenseChecker,
	}
}

// SetEventCallback sets the callback for broadcasting heartbeat events.
func (s *Service) SetEventCallback(cb EventCallback) {
	s.onEvent = cb
}

// SetAlertCallback sets the callback for alert/recovery events.
func (s *Service) SetAlertCallback(cb AlertCallback) {
	s.alertCallback = cb
}

// SetBaseURL sets the base URL for generating ping URLs and snippets.
func (s *Service) SetBaseURL(url string) {
	s.baseURL = url
}

// BaseURL returns the configured base URL.
func (s *Service) BaseURL() string {
	return s.baseURL
}

// --- CRUD ---

func (s *Service) CreateHeartbeat(ctx context.Context, input CreateHeartbeatInput, uuid string) (*Heartbeat, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	count, err := s.store.CountActiveHeartbeats(ctx)
	if err != nil {
		return nil, fmt.Errorf("count heartbeats: %w", err)
	}
	if !s.licenseChecker.CanCreateHeartbeat(count) {
		return nil, ErrLimitReached
	}

	h := &Heartbeat{
		UUID:            uuid,
		Name:            input.Name,
		Status:          StatusNew,
		AlertState:      AlertNormal,
		IntervalSeconds: input.IntervalSeconds,
		GraceSeconds:    input.GraceSeconds,
		Active:          true,
	}

	id, err := s.store.CreateHeartbeat(ctx, h)
	if err != nil {
		return nil, fmt.Errorf("create heartbeat: %w", err)
	}

	h.ID = id
	created, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.emitEvent("heartbeat.created", map[string]interface{}{
		"heartbeat_id": created.ID,
		"name":         created.Name,
		"uuid":         created.UUID,
		"status":       string(created.Status),
	})

	return created, nil
}

func (s *Service) GetHeartbeat(ctx context.Context, id int64) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}
	return h, nil
}

func (s *Service) ListHeartbeats(ctx context.Context, opts ListHeartbeatsOpts) ([]*Heartbeat, error) {
	return s.store.ListHeartbeats(ctx, opts)
}

func (s *Service) UpdateHeartbeat(ctx context.Context, id int64, input UpdateHeartbeatInput) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	if input.Name != nil && (len(*input.Name) == 0 || len(*input.Name) > MaxNameLength) {
		return nil, fmt.Errorf("%w: name must be 1-%d characters", ErrInvalidInput, MaxNameLength)
	}
	if input.IntervalSeconds != nil && (*input.IntervalSeconds < MinIntervalSeconds || *input.IntervalSeconds > MaxIntervalSeconds) {
		return nil, fmt.Errorf("%w: interval must be %d-%d seconds", ErrInvalidInput, MinIntervalSeconds, MaxIntervalSeconds)
	}
	if input.GraceSeconds != nil {
		maxGrace := h.IntervalSeconds
		if input.IntervalSeconds != nil {
			maxGrace = *input.IntervalSeconds
		}
		if *input.GraceSeconds < 0 || *input.GraceSeconds > maxGrace {
			return nil, fmt.Errorf("%w: grace must be 0-%d seconds", ErrInvalidInput, maxGrace)
		}
	}

	if err := s.store.UpdateHeartbeat(ctx, id, input); err != nil {
		return nil, err
	}

	return s.store.GetHeartbeatByID(ctx, id)
}

func (s *Service) DeleteHeartbeat(ctx context.Context, id int64) error {
	h, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return err
	}
	if h == nil {
		return ErrHeartbeatNotFound
	}

	if err := s.store.DeleteHeartbeat(ctx, id); err != nil {
		return err
	}

	s.emitEvent("heartbeat.deleted", map[string]interface{}{
		"heartbeat_id": id,
	})

	return nil
}

// --- Ping Processing ---

func (s *Service) ProcessPing(ctx context.Context, uuid string, sourceIP, httpMethod string, payload *string) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	now := time.Now()
	previousStatus := h.Status

	// Record ping
	storedPayload := s.filterPayload(payload)
	ping := &HeartbeatPing{
		HeartbeatID: h.ID,
		PingType:    PingSuccess,
		SourceIP:    sourceIP,
		HTTPMethod:  httpMethod,
		Payload:     storedPayload,
		Timestamp:   now,
	}
	if _, err := s.store.InsertPing(ctx, ping); err != nil {
		s.logger.Error("insert ping", "heartbeat_id", h.ID, "error", err)
	}

	// State transition
	newStatus := StatusUp
	deadline := now.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	h.LastPingAt = &now
	h.NextDeadlineAt = &deadline
	h.ConsecutiveSuccesses++
	h.ConsecutiveFailures = 0

	// Handle started → up (completion ping)
	if previousStatus == StatusStarted && h.CurrentRunStartedAt != nil {
		durationMs := now.Sub(*h.CurrentRunStartedAt).Milliseconds()
		h.LastDurationMs = &durationMs
		h.CurrentRunStartedAt = nil

		// Complete current execution
		exec, err := s.store.GetCurrentExecution(ctx, h.ID)
		if err == nil && exec != nil {
			if err := s.store.UpdateExecution(ctx, exec.ID, &now, &durationMs, nil, OutcomeSuccess, storedPayload); err != nil {
				s.logger.Error("update execution", "execution_id", exec.ID, "error", err)
			}
		}
	} else {
		// Create execution record for non-start/finish tracking
		exec := &HeartbeatExecution{
			HeartbeatID: h.ID,
			CompletedAt: &now,
			Outcome:     OutcomeSuccess,
			Payload:     storedPayload,
		}
		if _, err := s.store.InsertExecution(ctx, exec); err != nil {
			s.logger.Error("insert execution", "heartbeat_id", h.ID, "error", err)
		}
	}

	// Update state
	alertState := h.AlertState
	wasDown := previousStatus == StatusDown
	if wasDown {
		alertState = AlertNormal
	}

	if err := s.store.UpdateHeartbeatState(ctx, h.ID, newStatus, alertState,
		h.LastPingAt, h.NextDeadlineAt, h.CurrentRunStartedAt,
		h.LastExitCode, h.LastDurationMs,
		h.ConsecutiveFailures, h.ConsecutiveSuccesses); err != nil {
		return nil, fmt.Errorf("update heartbeat state: %w", err)
	}

	// Emit events
	s.emitEvent("heartbeat.ping_received", map[string]interface{}{
		"heartbeat_id": h.ID,
		"ping_type":    string(PingSuccess),
		"status":       string(newStatus),
	})

	if newStatus != previousStatus {
		s.emitEvent("heartbeat.status_changed", map[string]interface{}{
			"heartbeat_id": h.ID,
			"old_status":   string(previousStatus),
			"new_status":   string(newStatus),
		})
	}

	// Recovery alert
	if wasDown {
		s.emitEvent("heartbeat.recovery", map[string]interface{}{
			"heartbeat_id": h.ID,
			"name":         h.Name,
		})
		if s.alertCallback != nil {
			s.alertCallback(h, "recovery", map[string]interface{}{
				"heartbeat_id": h.ID,
				"name":         h.Name,
			})
		}
	}

	h.Status = newStatus
	h.AlertState = alertState
	return h, nil
}

func (s *Service) ProcessStartPing(ctx context.Context, uuid string, sourceIP, httpMethod string) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	now := time.Now()

	// Record ping
	ping := &HeartbeatPing{
		HeartbeatID: h.ID,
		PingType:    PingStart,
		SourceIP:    sourceIP,
		HTTPMethod:  httpMethod,
		Timestamp:   now,
	}
	if _, err := s.store.InsertPing(ctx, ping); err != nil {
		s.logger.Error("insert start ping", "heartbeat_id", h.ID, "error", err)
	}

	// Close any in-progress execution as timeout
	currentExec, err := s.store.GetCurrentExecution(ctx, h.ID)
	if err == nil && currentExec != nil {
		if err := s.store.UpdateExecution(ctx, currentExec.ID, &now, nil, nil, OutcomeTimeout, nil); err != nil {
			s.logger.Error("timeout previous execution", "execution_id", currentExec.ID, "error", err)
		}
	}

	// Create new execution
	exec := &HeartbeatExecution{
		HeartbeatID: h.ID,
		StartedAt:   &now,
		Outcome:     OutcomeInProgress,
	}
	if _, err := s.store.InsertExecution(ctx, exec); err != nil {
		s.logger.Error("insert start execution", "heartbeat_id", h.ID, "error", err)
	}

	// Update state
	newStatus := StatusStarted
	deadline := now.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	previousStatus := h.Status

	if err := s.store.UpdateHeartbeatState(ctx, h.ID, newStatus, h.AlertState,
		h.LastPingAt, &deadline, &now,
		h.LastExitCode, h.LastDurationMs,
		h.ConsecutiveFailures, h.ConsecutiveSuccesses); err != nil {
		return nil, fmt.Errorf("update heartbeat state: %w", err)
	}

	// Emit events
	s.emitEvent("heartbeat.ping_received", map[string]interface{}{
		"heartbeat_id": h.ID,
		"ping_type":    string(PingStart),
		"status":       string(newStatus),
	})

	if newStatus != previousStatus {
		s.emitEvent("heartbeat.status_changed", map[string]interface{}{
			"heartbeat_id": h.ID,
			"old_status":   string(previousStatus),
			"new_status":   string(newStatus),
		})
	}

	h.Status = newStatus
	h.CurrentRunStartedAt = &now
	h.NextDeadlineAt = &deadline
	return h, nil
}

func (s *Service) ProcessExitCodePing(ctx context.Context, uuid string, exitCode int, sourceIP, httpMethod string, payload *string) (*Heartbeat, error) {
	if exitCode < 0 || exitCode > 255 {
		return nil, ErrInvalidExitCode
	}

	h, err := s.store.GetHeartbeatByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	now := time.Now()
	previousStatus := h.Status

	// Record ping
	storedPayload := s.filterPayload(payload)
	ping := &HeartbeatPing{
		HeartbeatID: h.ID,
		PingType:    PingExitCode,
		ExitCode:    &exitCode,
		SourceIP:    sourceIP,
		HTTPMethod:  httpMethod,
		Payload:     storedPayload,
		Timestamp:   now,
	}
	if _, err := s.store.InsertPing(ctx, ping); err != nil {
		s.logger.Error("insert exit code ping", "heartbeat_id", h.ID, "error", err)
	}

	// Reset deadline (job did run)
	newStatus := StatusUp
	deadline := now.Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	h.LastPingAt = &now
	h.NextDeadlineAt = &deadline
	h.LastExitCode = &exitCode

	// Handle started → up (completion)
	var durationMs *int64
	if previousStatus == StatusStarted && h.CurrentRunStartedAt != nil {
		d := now.Sub(*h.CurrentRunStartedAt).Milliseconds()
		durationMs = &d
		h.LastDurationMs = &d
		h.CurrentRunStartedAt = nil
	}

	// Determine outcome and alert state
	var outcome ExecutionOutcome
	alertState := h.AlertState
	if exitCode == 0 {
		outcome = OutcomeSuccess
		h.ConsecutiveSuccesses++
		h.ConsecutiveFailures = 0
		if alertState == AlertAlerting {
			alertState = AlertNormal
		}
	} else {
		outcome = OutcomeFailure
		h.ConsecutiveFailures++
		h.ConsecutiveSuccesses = 0
		alertState = AlertAlerting
	}

	// Update or create execution record
	currentExec, _ := s.store.GetCurrentExecution(ctx, h.ID)
	if currentExec != nil {
		if err := s.store.UpdateExecution(ctx, currentExec.ID, &now, durationMs, &exitCode, outcome, storedPayload); err != nil {
			s.logger.Error("update execution", "execution_id", currentExec.ID, "error", err)
		}
	} else {
		exec := &HeartbeatExecution{
			HeartbeatID: h.ID,
			CompletedAt: &now,
			DurationMs:  durationMs,
			ExitCode:    &exitCode,
			Outcome:     outcome,
			Payload:     storedPayload,
		}
		if _, err := s.store.InsertExecution(ctx, exec); err != nil {
			s.logger.Error("insert execution", "heartbeat_id", h.ID, "error", err)
		}
	}

	// Update state
	if err := s.store.UpdateHeartbeatState(ctx, h.ID, newStatus, alertState,
		h.LastPingAt, h.NextDeadlineAt, h.CurrentRunStartedAt,
		h.LastExitCode, h.LastDurationMs,
		h.ConsecutiveFailures, h.ConsecutiveSuccesses); err != nil {
		return nil, fmt.Errorf("update heartbeat state: %w", err)
	}

	// Emit events
	s.emitEvent("heartbeat.ping_received", map[string]interface{}{
		"heartbeat_id": h.ID,
		"ping_type":    string(PingExitCode),
		"exit_code":    exitCode,
		"status":       string(newStatus),
	})

	if newStatus != previousStatus {
		s.emitEvent("heartbeat.status_changed", map[string]interface{}{
			"heartbeat_id": h.ID,
			"old_status":   string(previousStatus),
			"new_status":   string(newStatus),
		})
	}

	// Alert or recovery
	if exitCode != 0 {
		s.emitEvent("heartbeat.alert", map[string]interface{}{
			"heartbeat_id": h.ID,
			"name":         h.Name,
			"alert_type":   "exit_code_failure",
			"details":      fmt.Sprintf("exit code %d", exitCode),
		})
		if s.alertCallback != nil {
			s.alertCallback(h, "alert", map[string]interface{}{
				"heartbeat_id": h.ID,
				"name":         h.Name,
				"exit_code":    exitCode,
			})
		}
	} else if previousStatus == StatusDown || h.AlertState == AlertAlerting {
		s.emitEvent("heartbeat.recovery", map[string]interface{}{
			"heartbeat_id": h.ID,
			"name":         h.Name,
		})
		if s.alertCallback != nil {
			s.alertCallback(h, "recovery", map[string]interface{}{
				"heartbeat_id": h.ID,
				"name":         h.Name,
			})
		}
	}

	h.Status = newStatus
	h.AlertState = alertState
	return h, nil
}

// --- Deadline Checker ---

func (s *Service) StartDeadlineChecker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkDeadlines(ctx)
			}
		}
	}()
}

func (s *Service) checkDeadlines(ctx context.Context) {
	now := time.Now()
	overdue, err := s.store.ListOverdueHeartbeats(ctx, now)
	if err != nil {
		s.logger.Error("list overdue heartbeats", "error", err)
		return
	}

	for _, h := range overdue {
		previousStatus := h.Status

		alertMsg := "Heartbeat missed deadline"
		if previousStatus == StatusStarted {
			alertMsg = "Job started but never finished"
		}

		h.Status = StatusDown
		h.AlertState = AlertAlerting
		h.ConsecutiveFailures++
		h.ConsecutiveSuccesses = 0

		if err := s.store.UpdateHeartbeatState(ctx, h.ID, StatusDown, AlertAlerting,
			h.LastPingAt, h.NextDeadlineAt, h.CurrentRunStartedAt,
			h.LastExitCode, h.LastDurationMs,
			h.ConsecutiveFailures, h.ConsecutiveSuccesses); err != nil {
			s.logger.Error("update overdue heartbeat", "heartbeat_id", h.ID, "error", err)
			continue
		}

		// Mark in-progress execution as timeout
		if previousStatus == StatusStarted {
			exec, err := s.store.GetCurrentExecution(ctx, h.ID)
			if err == nil && exec != nil {
				if err := s.store.UpdateExecution(ctx, exec.ID, &now, nil, nil, OutcomeTimeout, nil); err != nil {
					s.logger.Error("timeout execution", "execution_id", exec.ID, "error", err)
				}
			}
		}

		s.emitEvent("heartbeat.status_changed", map[string]interface{}{
			"heartbeat_id": h.ID,
			"old_status":   string(previousStatus),
			"new_status":   string(StatusDown),
		})

		s.emitEvent("heartbeat.alert", map[string]interface{}{
			"heartbeat_id": h.ID,
			"name":         h.Name,
			"alert_type":   "deadline_missed",
			"details":      alertMsg,
		})

		if s.alertCallback != nil {
			s.alertCallback(h, "alert", map[string]interface{}{
				"heartbeat_id": h.ID,
				"name":         h.Name,
				"alert_type":   "deadline_missed",
				"message":      alertMsg,
			})
		}

		s.logger.Warn("heartbeat deadline missed",
			"heartbeat_id", h.ID,
			"name", h.Name,
			"previous_status", string(previousStatus))
	}
}

// --- Pause / Resume ---

func (s *Service) PauseHeartbeat(ctx context.Context, id int64) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	if err := s.store.PauseHeartbeat(ctx, id); err != nil {
		return nil, err
	}

	s.emitEvent("heartbeat.status_changed", map[string]interface{}{
		"heartbeat_id": id,
		"old_status":   string(h.Status),
		"new_status":   string(StatusPaused),
	})

	return s.store.GetHeartbeatByID(ctx, id)
}

func (s *Service) ResumeHeartbeat(ctx context.Context, id int64) (*Heartbeat, error) {
	h, err := s.store.GetHeartbeatByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if h == nil {
		return nil, ErrHeartbeatNotFound
	}

	if h.Status != StatusPaused {
		return nil, fmt.Errorf("%w: heartbeat is not paused", ErrInvalidInput)
	}

	deadline := time.Now().Add(time.Duration(h.IntervalSeconds+h.GraceSeconds) * time.Second)
	if err := s.store.ResumeHeartbeat(ctx, id, deadline); err != nil {
		return nil, err
	}

	s.emitEvent("heartbeat.status_changed", map[string]interface{}{
		"heartbeat_id": id,
		"old_status":   string(StatusPaused),
		"new_status":   string(StatusUp),
	})

	return s.store.GetHeartbeatByID(ctx, id)
}

// --- Pings & Executions listing ---

func (s *Service) ListPings(ctx context.Context, heartbeatID int64, opts ListPingsOpts) ([]*HeartbeatPing, int, error) {
	return s.store.ListPings(ctx, heartbeatID, opts)
}

func (s *Service) ListExecutions(ctx context.Context, heartbeatID int64, opts ListExecutionsOpts) ([]*HeartbeatExecution, int, error) {
	return s.store.ListExecutions(ctx, heartbeatID, opts)
}

// --- Helpers ---

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.onEvent != nil {
		s.onEvent(eventType, data)
	}
}

func (s *Service) filterPayload(payload *string) *string {
	if payload == nil {
		return nil
	}
	if !s.licenseChecker.CanStorePayload() {
		return nil
	}
	p := *payload
	if len(p) > MaxPayloadBytes {
		p = p[:MaxPayloadBytes]
	}
	return &p
}

func validateCreateInput(input CreateHeartbeatInput) error {
	if len(input.Name) == 0 || len(input.Name) > MaxNameLength {
		return fmt.Errorf("name must be 1-%d characters", MaxNameLength)
	}
	if input.IntervalSeconds < MinIntervalSeconds || input.IntervalSeconds > MaxIntervalSeconds {
		return fmt.Errorf("interval must be %d-%d seconds", MinIntervalSeconds, MaxIntervalSeconds)
	}
	if input.GraceSeconds < 0 || input.GraceSeconds > input.IntervalSeconds {
		return fmt.Errorf("grace must be 0-%d seconds", input.IntervalSeconds)
	}
	return nil
}
