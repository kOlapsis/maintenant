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

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAckTracker_Pending(t *testing.T) {
	type step struct {
		refuse bool
		seq    uint64
	}
	tests := []struct {
		name  string
		steps []step
		want  uint64
		owed  bool
	}{
		{name: "nothing received", owed: false},
		{name: "highest seq wins", steps: []step{{seq: 3}, {seq: 7}, {seq: 5}}, want: 7, owed: true},
		{name: "unnumbered events are ignored", steps: []step{{seq: 0}, {seq: 0}}, owed: false},
		{name: "snapshots between spooled rows do not move the ack", steps: []step{{seq: 40}, {seq: 0}, {seq: 41}, {seq: 0}}, want: 41, owed: true},
		{
			name:  "refusal freezes the ack below the refused row",
			steps: []step{{seq: 10}, {seq: 11}, {refuse: true, seq: 12}, {seq: 13}, {seq: 14}},
			want:  11, owed: true,
		},
		{
			name:  "a lower refusal while frozen lowers the freeze",
			steps: []step{{seq: 10}, {refuse: true, seq: 14}, {refuse: true, seq: 12}, {seq: 13}},
			want:  10, owed: true,
		},
		{
			name:  "unnumbered refusal does not freeze",
			steps: []step{{seq: 10}, {refuse: true, seq: 0}, {seq: 11}},
			want:  11, owed: true,
		},
		{
			name:  "replay from the refused row unfreezes",
			steps: []step{{seq: 10}, {refuse: true, seq: 12}, {seq: 13}, {seq: 11}, {seq: 12}, {seq: 13}},
			want:  13, owed: true,
		},
		{
			name:  "a replay that is refused again stays frozen",
			steps: []step{{seq: 10}, {refuse: true, seq: 12}, {seq: 11}, {refuse: true, seq: 12}, {seq: 13}},
			want:  11, owed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newAckTracker(time.Now())
			for _, s := range tt.steps {
				if s.refuse {
					a.refuse(s.seq)
				} else {
					a.accept(s.seq)
				}
			}
			got, owed := a.pending()
			assert.Equal(t, tt.owed, owed)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAckTracker_ResumesAfterReplayBelowLastAck(t *testing.T) {
	now := time.Now()
	a := newAckTracker(now)
	a.accept(10)
	a.sent(10, now)
	a.refuse(12)
	a.accept(9)

	_, owed := a.pending()
	assert.False(t, owed, "a replay behind the last ack owes nothing")

	a.accept(12)
	got, owed := a.pending()
	assert.True(t, owed)
	assert.Equal(t, uint64(12), got)
}

func TestAckTracker_AfterEventWaitsForCountOrInterval(t *testing.T) {
	start := time.Now()
	a := newAckTracker(start)

	a.accept(1)
	_, owed := a.afterEvent(start.Add(time.Second))
	assert.False(t, owed, "one event within the interval waits")

	for seq := uint64(2); seq <= ackEvery; seq++ {
		a.accept(seq)
	}
	got, owed := a.afterEvent(start.Add(time.Second))
	assert.True(t, owed, "the count trigger fires")
	assert.Equal(t, uint64(ackEvery), got)
	a.sent(got, start.Add(time.Second))

	a.accept(ackEvery + 1)
	got, owed = a.afterEvent(start.Add(time.Second + ackInterval))
	assert.True(t, owed, "the interval trigger fires")
	assert.Equal(t, uint64(ackEvery+1), got)
}

func TestAckTracker_TimerAcksTheTailOnlyWhenNew(t *testing.T) {
	now := time.Now()
	a := newAckTracker(now)

	_, owed := a.pending()
	assert.False(t, owed, "no ack before anything arrived")

	a.accept(0)
	_, owed = a.pending()
	assert.False(t, owed, "a snapshot alone owes no ack")

	a.accept(5)
	_, owed = a.afterEvent(now)
	assert.False(t, owed, "the tail of a burst is not acked on arrival")

	got, owed := a.pending()
	assert.True(t, owed, "the timer acks the tail")
	assert.Equal(t, uint64(5), got)
	a.sent(got, now)

	_, owed = a.pending()
	assert.False(t, owed, "nothing new, no ack")
}
