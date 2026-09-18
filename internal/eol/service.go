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

package eol

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/hoststat"
	"github.com/kolapsis/maintenant/internal/uid"
)

// AlertType is the alert type every host end-of-support alert carries.
const AlertType = "os_eol"

// ReasonAgentTooOld marks an agent that never reported its operating system.
const ReasonAgentTooOld = "agent_too_old"

var (
	startDelay      = 30 * time.Second
	refreshInterval = 24 * time.Hour
)

// AgentStore reads the agents to evaluate and writes the server's own identity.
type AgentStore interface {
	ListAgentsForEOL(ctx context.Context) ([]agent.Agent, error)
	UpdateAgentOS(ctx context.Context, agentID string, os agent.OSIdentity, reportedAt time.Time) (bool, error)
}

// Deps carries what the service needs; a nil Fetcher disables the refresh.
type Deps struct {
	Store       AgentStore
	Fetcher     *Fetcher
	Emit        func(alert.Event)
	OnChanged   func(ctx context.Context, agentID string)
	ReadLocalOS func() hoststat.OSRelease
	Now         func() time.Time
	Logger      *slog.Logger
}

// TableStatus describes the support table currently in use.
type TableStatus struct {
	Source           string    `json:"source"`
	FetchedAt        time.Time `json:"fetched_at"`
	RefreshEnabled   bool      `json:"refresh_enabled"`
	LastRefreshError string    `json:"last_refresh_error"`
}

// HostSupport is the support state of one host, as served to the interface.
type HostSupport struct {
	AgentID    string
	Hostname   string
	Label      string
	IsLocal    bool
	Runtime    string
	Identity   Identity
	Support    Support
	ReportedAt *time.Time
	LastSeenAt *time.Time
}

// Service evaluates host operating systems against the support table and keeps that table fresh.
type Service struct {
	store       AgentStore
	fetcher     *Fetcher
	emit        func(alert.Event)
	onChanged   func(ctx context.Context, agentID string)
	readLocalOS func() hoststat.OSRelease
	now         func() time.Time
	logger      *slog.Logger

	mu               sync.RWMutex
	table            Table
	lastRefreshError string

	sevMu        sync.Mutex
	lastSeverity map[string]string
}

// New builds a service over the embedded support table.
func New(deps Deps) (*Service, error) {
	if deps.Store == nil {
		return nil, errors.New("eol: no store")
	}
	if deps.Emit == nil {
		return nil, errors.New("eol: no emit")
	}
	table, err := LoadEmbedded()
	if err != nil {
		return nil, err
	}
	s := &Service{
		store:        deps.Store,
		fetcher:      deps.Fetcher,
		emit:         deps.Emit,
		onChanged:    deps.OnChanged,
		readLocalOS:  deps.ReadLocalOS,
		now:          deps.Now,
		logger:       deps.Logger,
		table:        table,
		lastSeverity: make(map[string]string),
	}
	if s.readLocalOS == nil {
		s.readLocalOS = hoststat.ReadOSRelease
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	return s, nil
}

// Current returns the support table in use.
func (s *Service) Current() Table {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.table
}

// Status reports where the support table comes from and how the last refresh went.
func (s *Service) Status() TableStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return TableStatus{
		Source:           s.table.Source,
		FetchedAt:        s.table.FetchedAt,
		RefreshEnabled:   s.fetcher != nil,
		LastRefreshError: s.lastRefreshError,
	}
}

// Refresh downloads the support cycles and replaces the table in one go; on failure the current table stands.
func (s *Service) Refresh(ctx context.Context) error {
	if s.fetcher == nil {
		return nil
	}
	table, err := s.fetcher.Fetch(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastRefreshError = err.Error()
		s.mu.Unlock()
		return fmt.Errorf("refresh eol table: %w", err)
	}
	s.mu.Lock()
	s.table = table
	s.lastRefreshError = ""
	s.mu.Unlock()
	s.logger.Info("os end-of-support table refreshed", "source", table.Source, "fetched_at", table.FetchedAt)
	return nil
}

// SeedSeverities restores the last emitted severity per host from the active alerts.
func (s *Service) SeedSeverities(active []alert.Alert) {
	s.sevMu.Lock()
	defer s.sevMu.Unlock()
	for _, a := range active {
		if a.Source != alert.SourceHost || a.AlertType != AlertType {
			continue
		}
		s.lastSeverity[a.EntityID] = a.Severity
	}
}

// IdentityOf reads the operating system an agent reported, flagging as agent_too_old
// any agent but the sentinel that never reported one.
func IdentityOf(a agent.Agent) Identity {
	identity := Identity{
		ID:                a.OSID,
		VersionID:         a.OSVersionID,
		PrettyName:        a.OSPrettyName,
		Source:            a.OSSource,
		UnavailableReason: a.OSUnavailableReason,
	}
	if identity.Source == "" && identity.UnavailableReason == "" && identity.ID == "" && a.AgentID != uid.LocalAgent {
		identity.UnavailableReason = ReasonAgentTooOld
	}
	return identity
}

// EvaluateAgent re-evaluates a single host.
func (s *Service) EvaluateAgent(ctx context.Context, agentID string) error {
	agents, err := s.store.ListAgentsForEOL(ctx)
	if err != nil {
		return fmt.Errorf("evaluate agent %s: %w", agentID, err)
	}
	table := s.Current()
	now := s.now()
	for _, a := range agents {
		if a.AgentID == agentID {
			s.evaluate(a, table, now)
			return nil
		}
	}
	s.forget(agentID, now)
	return nil
}

// EvaluateAll re-evaluates every active host and resolves the hosts that left the fleet.
func (s *Service) EvaluateAll(ctx context.Context) error {
	agents, err := s.store.ListAgentsForEOL(ctx)
	if err != nil {
		return fmt.Errorf("evaluate hosts: %w", err)
	}
	table := s.Current()
	now := s.now()

	present := make(map[string]struct{}, len(agents))
	for _, a := range agents {
		present[a.AgentID] = struct{}{}
		s.evaluate(a, table, now)
	}

	s.sevMu.Lock()
	var gone []string
	for id := range s.lastSeverity {
		if _, ok := present[id]; !ok {
			gone = append(gone, id)
		}
	}
	s.sevMu.Unlock()

	for _, id := range gone {
		s.forget(id, now)
	}
	return nil
}

// HostSupports returns the support state of every active host, worst first.
func (s *Service) HostSupports(ctx context.Context) ([]HostSupport, error) {
	agents, err := s.store.ListAgentsForEOL(ctx)
	if err != nil {
		return nil, fmt.Errorf("list host support: %w", err)
	}
	table := s.Current()
	now := s.now()

	hosts := make([]HostSupport, 0, len(agents))
	for _, a := range agents {
		identity := IdentityOf(a)
		host := HostSupport{
			AgentID:    a.AgentID,
			Hostname:   a.Hostname,
			Label:      a.Label,
			IsLocal:    a.AgentID == uid.LocalAgent,
			Runtime:    a.DetectedRuntime,
			Identity:   identity,
			Support:    Evaluate(identity, table, now),
			ReportedAt: a.OSReportedAt,
			LastSeenAt: a.LastSeenAt,
		}
		hosts = append(hosts, host)
	}

	sort.SliceStable(hosts, func(i, j int) bool {
		ri, rj := stateRank(hosts[i].Support.State), stateRank(hosts[j].Support.State)
		if ri != rj {
			return ri < rj
		}
		if ri <= 1 {
			di, dj := daysOf(hosts[i].Support), daysOf(hosts[j].Support)
			if di != dj {
				return di < dj
			}
		}
		return displayName(hosts[i]) < displayName(hosts[j])
	})
	return hosts, nil
}

// CountStates counts hosts per state, security_only folded into supported.
func CountStates(hosts []HostSupport) map[string]int {
	counts := map[string]int{"ended": 0, "ending_soon": 0, "unknown": 0, "untracked": 0, "supported": 0}
	for _, h := range hosts {
		switch h.Support.State {
		case StateEnded:
			counts["ended"]++
		case StateEndingSoon:
			counts["ending_soon"]++
		case StateUnknown:
			counts["unknown"]++
		case StateUntracked:
			counts["untracked"]++
		default:
			counts["supported"]++
		}
	}
	return counts
}

// Start refreshes the table, re-reads the server's own os-release and evaluates every host, daily.
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("starting os end-of-support service",
		"refresh_enabled", s.fetcher != nil, "interval", refreshInterval)

	select {
	case <-ctx.Done():
		return nil
	case <-time.After(startDelay):
	}
	s.runPass(ctx)

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.runPass(ctx)
		}
	}
}

// RefreshLocalOS re-reads the server's own os-release and stores it under the local sentinel.
func (s *Service) RefreshLocalOS(ctx context.Context) error {
	rel := s.readLocalOS()
	identity := agent.OSIdentity{
		ID:                rel.ID,
		VersionID:         rel.VersionID,
		PrettyName:        rel.PrettyName,
		Source:            rel.Source,
		UnavailableReason: rel.UnavailableReason,
	}
	if _, err := s.store.UpdateAgentOS(ctx, uid.LocalAgent, identity, s.now()); err != nil {
		return fmt.Errorf("store local os identity: %w", err)
	}
	return nil
}

func (s *Service) runPass(ctx context.Context) {
	if s.fetcher != nil {
		if err := s.Refresh(ctx); err != nil {
			s.logger.Warn("os end-of-support table refresh failed, keeping current table", "error", err)
		}
	}

	if err := s.RefreshLocalOS(ctx); err != nil {
		s.logger.Warn("local os identity not stored", "error", err)
	}

	if err := s.EvaluateAll(ctx); err != nil {
		s.logger.Warn("host end-of-support evaluation failed", "error", err)
	}
}

func (s *Service) evaluate(a agent.Agent, table Table, now time.Time) {
	identity := IdentityOf(a)
	support := Evaluate(identity, table, now)
	severity := severityFor(support.State)
	base := hostEvent(a, identity, support, now)

	s.sevMu.Lock()
	previous := s.lastSeverity[a.AgentID]
	if severity == "" {
		delete(s.lastSeverity, a.AgentID)
	} else {
		s.lastSeverity[a.AgentID] = severity
	}
	s.sevMu.Unlock()

	if severity == "" {
		s.emit(recovery(base))
		return
	}
	// The engine escalates a rising severity in place but ignores a falling one.
	if previous == alert.SeverityCritical && severity == alert.SeverityWarning {
		s.emit(recovery(base))
	}
	base.Severity = severity
	s.emit(base)
}

func (s *Service) forget(agentID string, now time.Time) {
	s.sevMu.Lock()
	_, tracked := s.lastSeverity[agentID]
	delete(s.lastSeverity, agentID)
	s.sevMu.Unlock()
	if !tracked {
		return
	}
	s.emit(alert.Event{
		Source:     alert.SourceHost,
		AlertType:  AlertType,
		Severity:   alert.SeverityInfo,
		IsRecover:  true,
		Message:    "Host is no longer monitored",
		EntityType: "agent",
		EntityID:   agentID,
		EntityName: agentID,
		Timestamp:  now,
	})
}

func hostEvent(a agent.Agent, identity Identity, support Support, now time.Time) alert.Event {
	name := a.Label
	if name == "" {
		name = a.Hostname
	}
	if name == "" {
		name = a.AgentID
	}
	return alert.Event{
		Source:     alert.SourceHost,
		AlertType:  AlertType,
		EntityType: "agent",
		EntityID:   a.AgentID,
		EntityName: name,
		Message:    supportMessage(identity, support),
		Details:    supportDetails(identity, support),
		Timestamp:  now,
	}
}

func recovery(evt alert.Event) alert.Event {
	evt.Severity = alert.SeverityInfo
	evt.IsRecover = true
	return evt
}

func severityFor(state SupportState) string {
	switch state {
	case StateEnded:
		return alert.SeverityCritical
	case StateEndingSoon:
		return alert.SeverityWarning
	}
	return ""
}

func supportMessage(identity Identity, support Support) string {
	name := osName(identity)
	switch support.State {
	case StateUnknown:
		return "Host operating system not reported"
	case StateUntracked:
		return fmt.Sprintf("%s: end of support is not tracked", name)
	}

	days := 0
	if support.DaysRemaining != nil {
		days = *support.DaysRemaining
	}
	switch {
	case days < 0:
		return fmt.Sprintf("%s: free security support ended on %s (%s ago)", name, support.SecurityUntil, plural(-days, "day"))
	case days == 0:
		return fmt.Sprintf("%s: free security support ends today", name)
	default:
		return fmt.Sprintf("%s: free security support ends on %s (in %s)", name, support.SecurityUntil, plural(days, "day"))
	}
}

func supportDetails(identity Identity, support Support) map[string]any {
	details := map[string]any{
		"os_id":          identity.ID,
		"os_version_id":  identity.VersionID,
		"os_pretty_name": identity.PrettyName,
		"product":        support.Product,
		"cycle":          support.Cycle,
		"state":          string(support.State),
		"active_until":   dateValue(support.ActiveUntil),
		"security_until": dateValue(support.SecurityUntil),
		"extended_until": dateValue(support.ExtendedUntil),
		"days_remaining": nil,
		"table_source":   support.TableSource,
	}
	if support.DaysRemaining != nil {
		details["days_remaining"] = *support.DaysRemaining
	}
	return details
}

func dateValue(d *Date) any {
	if d == nil {
		return nil
	}
	return d.String()
}

func osName(identity Identity) string {
	if identity.PrettyName != "" {
		return identity.PrettyName
	}
	if identity.VersionID == "" {
		return identity.ID
	}
	return identity.ID + " " + identity.VersionID
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func stateRank(state SupportState) int {
	switch state {
	case StateEnded:
		return 0
	case StateEndingSoon:
		return 1
	case StateUnknown:
		return 2
	case StateUntracked:
		return 3
	case StateSecurityOnly:
		return 4
	}
	return 5
}

func daysOf(support Support) int {
	if support.DaysRemaining == nil {
		return 0
	}
	return *support.DaysRemaining
}

func displayName(h HostSupport) string {
	if h.Label != "" {
		return h.Label
	}
	return h.Hostname
}
