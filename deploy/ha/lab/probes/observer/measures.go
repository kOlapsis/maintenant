// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See deploy/ha/LICENSE.

package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const localAgentID = "00000000-0000-0000-0000-000000000000"

type alertSeen struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	EntityID string `json:"entity_id"`
	AgentID  string `json:"agent_id,omitempty"`
	Severity string `json:"severity"`
}

type hostSeen struct {
	AgentID   string `json:"agent_id"`
	Hostname  string `json:"hostname,omitempty"`
	Label     string `json:"label,omitempty"`
	IsLocal   bool   `json:"is_local,omitempty"`
	Available bool   `json:"available"`
}

func (h hostSeen) local() bool {
	return h.IsLocal || h.AgentID == "" || h.AgentID == localAgentID
}

func (h hostSeen) name() string {
	switch {
	case h.Label != "":
		return h.Label
	case h.Hostname != "":
		return h.Hostname
	default:
		return h.AgentID
	}
}

type observation struct {
	Seq       uint64      `json:"seq"`
	Wall      string      `json:"wall"`
	Epoch     string      `json:"epoch"`
	MonoNS    int64       `json:"mono_ns"`
	Reachable bool        `json:"reachable"`
	Hosts     []hostSeen  `json:"hosts"`
	Alerts    []alertSeen `json:"alerts"`
	Error     string      `json:"error,omitempty"`
}

type induced struct {
	AgentOffline     int `json:"agent_offline"`
	HeartbeatOverdue int `json:"heartbeat_overdue"`
	EndpointDown     int `json:"endpoint_down"`
	Other            int `json:"other"`
}

type agentsMeasure struct {
	Expected      []string `json:"expected"`
	Reconnected   int      `json:"reconnected"`
	Missing       []string `json:"missing"`
	MaxReconnectS float64  `json:"max_reconnect_s"`
	NewAgentIDs   []string `json:"new_agent_ids"`
}

func namesMatch(wanted, label, hostname string) bool {
	short, _, _ := strings.Cut(hostname, ".")
	for _, candidate := range []string{label, hostname, short} {
		if candidate != "" && strings.EqualFold(candidate, wanted) {
			return true
		}
	}
	return false
}

func faultedAgents(observations []observation, names []string) map[string]bool {
	ids := map[string]bool{}
	for _, o := range observations {
		for _, h := range o.Hosts {
			if h.local() {
				continue
			}
			for _, n := range names {
				if namesMatch(n, h.Label, h.Hostname) {
					ids[h.AgentID] = true
				}
			}
		}
	}
	return ids
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func wallOf(o observation) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, o.Wall)
	return at, err == nil
}

func flowing(o observation, faulted map[string]bool) bool {
	if !o.Reachable {
		return false
	}
	for _, h := range o.Hosts {
		if h.Available && !h.local() && !faulted[h.AgentID] {
			return true
		}
	}
	return false
}

func telemetryGap(observations []observation, from, to time.Time, faulted map[string]bool) (float64, error) {
	var lastFlowing time.Time
	var widest float64
	haveFlowing := false
	covered := false

	for _, o := range observations {
		at, ok := wallOf(o)
		if !ok {
			continue
		}
		if !at.Before(to) {
			covered = true
		}
		if at.Before(from) || at.After(to) || !flowing(o, faulted) {
			continue
		}
		since := from
		if haveFlowing {
			since = lastFlowing
		}
		widest = math.Max(widest, at.Sub(since).Seconds())
		lastFlowing = at
		haveFlowing = true
	}
	if !haveFlowing {
		return 0, errors.New("no telemetry from a remote agent that stayed up reached the service in the window")
	}
	if covered {
		widest = math.Max(widest, to.Sub(lastFlowing).Seconds())
	}
	return widest, nil
}

func lastReachableAtOrBefore(observations []observation, at time.Time) (observation, bool) {
	var found observation
	ok := false
	for _, o := range observations {
		w, valid := wallOf(o)
		if !valid || w.After(at) || !o.Reachable {
			continue
		}
		found = o
		ok = true
	}
	return found, ok
}

func alertsInduced(observations []observation, from, to time.Time, faulted map[string]bool) (induced, []string, error) {
	var counts induced
	ids := []string{}

	before, ok := lastReachableAtOrBefore(observations, from)
	if !ok {
		return counts, ids, fmt.Errorf("no reachable observation at or before %s: nothing to compare the alerts against",
			from.UTC().Format(time.RFC3339Nano))
	}
	baseline := map[string]bool{}
	for _, a := range before.Alerts {
		baseline[a.Source+"/"+a.EntityID] = true
	}

	counted := map[string]bool{}
	for _, o := range observations {
		at, valid := wallOf(o)
		if !valid || !at.After(from) || at.After(to) {
			continue
		}
		for _, a := range o.Alerts {
			if counted[a.ID] || baseline[a.Source+"/"+a.EntityID] {
				continue
			}
			if a.Source == "agent" && faulted[a.EntityID] {
				continue
			}
			counted[a.ID] = true
			ids = append(ids, a.ID)
			switch a.Source {
			case "agent":
				counts.AgentOffline++
			case "heartbeat":
				counts.HeartbeatOverdue++
			case "endpoint":
				counts.EndpointDown++
			default:
				counts.Other++
			}
		}
	}
	sort.Strings(ids)
	return counts, ids, nil
}

func agentsReconnect(observations []observation, from, to time.Time, faulted map[string]bool) (agentsMeasure, error) {
	m := agentsMeasure{Expected: []string{}, Missing: []string{}, NewAgentIDs: []string{}}

	before, ok := lastReachableAtOrBefore(observations, from)
	if !ok {
		return m, fmt.Errorf("no reachable observation at or before %s: no agent to expect back",
			from.UTC().Format(time.RFC3339Nano))
	}

	names := map[string]string{}
	var expected []string
	for _, h := range before.Hosts {
		if h.local() || !h.Available || faulted[h.AgentID] {
			continue
		}
		expected = append(expected, h.AgentID)
		names[h.AgentID] = h.name()
	}

	seenBefore := map[string]bool{}
	for _, o := range observations {
		at, valid := wallOf(o)
		if !valid || at.After(from) {
			continue
		}
		for _, h := range o.Hosts {
			seenBefore[h.AgentID] = true
		}
	}

	outage := map[string]time.Time{}
	back := map[string]time.Time{}
	last := map[string]bool{}
	fresh := map[string]bool{}
	reachable := 0
	for _, o := range observations {
		at, valid := wallOf(o)
		if !valid || !at.After(from) || at.After(to) || !o.Reachable {
			continue
		}
		reachable++
		up := map[string]bool{}
		for _, h := range o.Hosts {
			if h.Available {
				up[h.AgentID] = true
			}
			if !h.local() && !seenBefore[h.AgentID] {
				fresh[h.AgentID] = true
			}
		}
		for _, id := range expected {
			_, out := outage[id]
			switch {
			case !up[id] && !out:
				outage[id] = at
			case up[id] && out && back[id].IsZero():
				back[id] = at
			}
		}
		last = up
	}
	if reachable == 0 {
		return m, fmt.Errorf("no reachable observation between %s and %s: the agents cannot be judged",
			from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	}

	for _, id := range expected {
		m.Expected = append(m.Expected, names[id])
		if !last[id] {
			m.Missing = append(m.Missing, names[id])
			continue
		}
		m.Reconnected++
		if start, out := outage[id]; out && !back[id].IsZero() {
			m.MaxReconnectS = math.Max(m.MaxReconnectS, back[id].Sub(start).Seconds())
		}
	}
	sort.Strings(m.Expected)
	sort.Strings(m.Missing)
	m.NewAgentIDs = append(m.NewAgentIDs, sortedKeys(fresh)...)
	return m, nil
}

type containerSeen struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	AgentID       string  `json:"agent_id"`
	AgentHostname *string `json:"agent_hostname"`
	AgentLabel    *string `json:"agent_label"`
	Archived      bool    `json:"archived"`
}

func resolveContainer(all []containerSeen, name, host string) (string, error) {
	var hits []containerSeen
	for _, c := range all {
		if strings.TrimPrefix(c.Name, "/") != name || c.Archived {
			continue
		}
		if c.AgentID == "" || c.AgentID == localAgentID {
			continue
		}
		if host != "" && !namesMatch(host, deref(c.AgentLabel), deref(c.AgentHostname)) {
			continue
		}
		hits = append(hits, c)
	}
	switch len(hits) {
	case 0:
		return "", fmt.Errorf("no live container named %q on a remote agent", name)
	case 1:
		return hits[0].ID, nil
	default:
		var where []string
		for _, c := range hits {
			where = append(where, c.ID+"@"+c.AgentID)
		}
		return "", fmt.Errorf("%d containers named %q on remote agents (%s): pass -container-host", len(hits), name, strings.Join(where, ", "))
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func samplesLost(stamps []time.Time, from, to time.Time) (int, error) {
	if len(stamps) < 3 {
		return 0, fmt.Errorf("history holds %d points, too few to read a cadence", len(stamps))
	}
	sorted := append([]time.Time(nil), stamps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })

	gaps := make([]time.Duration, 0, len(sorted)-1)
	for i := 1; i < len(sorted); i++ {
		gaps = append(gaps, sorted[i].Sub(sorted[i-1]))
	}
	ordered := append([]time.Duration(nil), gaps...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	cadence := ordered[len(ordered)/2]
	if cadence <= 0 {
		return 0, errors.New("history cadence reads as zero")
	}

	inWindow := func(t time.Time) bool { return !t.Before(from) && !t.After(to) }
	lost := 0
	for i, g := range gaps {
		missing := int(math.Round(float64(g)/float64(cadence))) - 1
		for k := 1; k <= missing; k++ {
			if inWindow(sorted[i].Add(g * time.Duration(k) / time.Duration(missing+1))) {
				lost++
			}
		}
	}
	for due := sorted[len(sorted)-1].Add(cadence); !due.After(to); due = due.Add(cadence) {
		if inWindow(due) {
			lost++
		}
	}
	return lost, nil
}
