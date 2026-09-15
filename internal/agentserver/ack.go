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

package agentserver

import "time"

const (
	ackEvery    = 100
	ackInterval = 5 * time.Second
	ackTick     = 2 * time.Second
)

// ackTracker decides which event sequence a stream may acknowledge.
type ackTracker struct {
	accepted uint64
	highest  uint64
	refused  uint64
	acked    uint64
	lastAck  time.Time
}

func newAckTracker(now time.Time) *ackTracker {
	return &ackTracker{lastAck: now}
}

// accept records an event the server took. An unnumbered event never moves the ack.
func (a *ackTracker) accept(seq uint64) {
	a.accepted++
	if seq == 0 {
		return
	}
	if a.refused > 0 {
		if seq > a.refused {
			return
		}
		a.refused = 0
		a.highest = seq
		return
	}
	a.highest = max(a.highest, seq)
}

// refuse freezes the ack below seq until the agent replays from there.
func (a *ackTracker) refuse(seq uint64) {
	if seq == 0 {
		return
	}
	if a.refused == 0 || seq < a.refused {
		a.refused = seq
	}
}

// afterEvent returns the sequence to acknowledge once an event is handled, if one is owed.
func (a *ackTracker) afterEvent(now time.Time) (uint64, bool) {
	if a.accepted%ackEvery != 0 && now.Sub(a.lastAck) < ackInterval {
		return 0, false
	}
	return a.pending()
}

// pending returns the sequence to acknowledge when it is past the last ack.
func (a *ackTracker) pending() (uint64, bool) {
	if a.highest <= a.acked {
		return 0, false
	}
	return a.highest, true
}

func (a *ackTracker) sent(seq uint64, now time.Time) {
	a.acked = seq
	a.lastAck = now
}
