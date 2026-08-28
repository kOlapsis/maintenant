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

package resource

import (
	"sync"
	"time"

	"github.com/kolapsis/maintenant/internal/hoststat"
)

// HostSampleTTL bounds how long a remote agent's host sample is considered
// fresh. Beyond it the host is reported with no current metrics (the agent has
// likely disconnected). Three sample intervals of slack.
const HostSampleTTL = 35 * time.Second

// hostRegistry holds the latest host-level sample per remote agent, keyed by
// agent_id. The local server host is never stored here — it is read live from
// the HostStatReader. Concurrency-safe.
type hostRegistry struct {
	mu      sync.RWMutex
	samples map[string]*HostSample
}

func newHostRegistry() *hostRegistry {
	return &hostRegistry{samples: make(map[string]*HostSample)}
}

func (r *hostRegistry) put(s *HostSample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples[s.AgentID] = s
}

func (r *hostRegistry) get(agentID string) *HostSample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.samples[agentID]
}

func (r *hostRegistry) list() []*HostSample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*HostSample, 0, len(r.samples))
	for _, s := range r.samples {
		out = append(out, s)
	}
	return out
}

// RecordHostSample stores the latest host-level sample for a remote agent.
// A non-empty AgentID is required; samples with an empty AgentID are ignored
// (the local server host is read live, never stored).
func (s *Service) RecordHostSample(sample *HostSample) {
	if sample == nil || sample.AgentID == "" {
		return
	}
	s.hosts.put(sample)
}

// localHostSample builds the local server host sample from the live reader.
func (s *Service) localHostSample(now time.Time) *HostSample {
	hs := s.GetHostStat()
	total, used := hoststat.DiskUsage("/")
	return &HostSample{
		AgentID:    "",
		CPUPercent: hs.CPUPercent(),
		MemUsed:    hs.MemUsed(),
		MemTotal:   hs.MemTotal(),
		DiskTotal:  total,
		DiskUsed:   used,
		Timestamp:  now,
	}
}

// HostStatForAgent returns the latest host sample for the given host. The local
// server uses the empty-string key and is always available. A remote agent's
// sample is returned only when fresher than HostSampleTTL; nil otherwise.
func (s *Service) HostStatForAgent(agentID string) *HostSample {
	if agentID == "" {
		return s.localHostSample(time.Now())
	}
	sample := s.hosts.get(agentID)
	if sample == nil || time.Since(sample.Timestamp) > HostSampleTTL {
		return nil
	}
	return sample
}

// ListHostStats returns the latest sample for every known host: the local
// server first, then every remote agent that has reported (including stale
// ones — callers decide how to present staleness via the Timestamp).
func (s *Service) ListHostStats() []*HostSample {
	out := []*HostSample{s.localHostSample(time.Now())}
	return append(out, s.hosts.list()...)
}

// IsHostSampleFresh reports whether a host sample is recent enough to trust.
func IsHostSampleFresh(sample *HostSample, now time.Time) bool {
	return sample != nil && now.Sub(sample.Timestamp) <= HostSampleTTL
}
