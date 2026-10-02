// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package anomaly

import (
	"context"
	"sort"

	model "github.com/kolapsis/maintenant/internal/anomaly"
)

// fakeStore copies on read and write like the database, so a mutation nobody persists does not pass unnoticed.
type fakeStore struct {
	hourly    []model.HourlyResourceRow
	snapshots []model.SnapshotRow
	baselines map[string]map[int]model.Baseline // series key -> bucket -> baseline
	states    map[string]*model.SeriesState
	events    []*model.AnomalyEvent
	settings  *model.Settings
	nextID    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		baselines: map[string]map[int]model.Baseline{},
		states:    map[string]*model.SeriesState{},
	}
}

func (f *fakeStore) ListContainerHourly(_ context.Context, since int64) ([]model.HourlyResourceRow, error) {
	var out []model.HourlyResourceRow
	for _, r := range f.hourly {
		if r.Bucket >= since {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) ListContainerSnapshotsSince(_ context.Context, since int64) ([]model.SnapshotRow, error) {
	var out []model.SnapshotRow
	for _, r := range f.snapshots {
		if r.Timestamp >= since {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) GetSettings(_ context.Context) (*model.Settings, error) { return f.settings, nil }

func (f *fakeStore) UpdateSettings(_ context.Context, s model.Settings) error {
	f.settings = &s
	return nil
}

func (f *fakeStore) GetBaseline(_ context.Context, key model.SeriesKey, bucket int) (*model.Baseline, error) {
	m := f.baselines[keyStr(key)]
	if b, ok := m[bucket]; ok {
		cp := b
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeStore) UpsertBaseline(_ context.Context, b model.Baseline) error {
	k := keyStr(b.SeriesKey)
	if f.baselines[k] == nil {
		f.baselines[k] = map[int]model.Baseline{}
	}
	f.baselines[k][b.Bucket] = b
	return nil
}

func (f *fakeStore) ListBaselines(_ context.Context, key model.SeriesKey) ([]model.Baseline, error) {
	m := f.baselines[keyStr(key)]
	out := make([]model.Baseline, 0, len(m))
	for _, b := range m {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bucket < out[j].Bucket })
	return out, nil
}

func (f *fakeStore) DeleteBaselinesForScope(_ context.Context, scopeType, scopeID string) error {
	for k := range f.baselines {
		// key is scopeType|scopeID|metric|dimension
		if hasScopePrefix(k, scopeType, scopeID) {
			delete(f.baselines, k)
		}
	}
	return nil
}

func hasScopePrefix(key, scopeType, scopeID string) bool {
	prefix := scopeType + "|" + scopeID + "|"
	return len(key) >= len(prefix) && key[:len(prefix)] == prefix
}

func (f *fakeStore) UpsertSeriesState(_ context.Context, s *model.SeriesState) error {
	cp := *s
	f.states[keyStr(s.SeriesKey)] = &cp
	return nil
}

func (f *fakeStore) GetSeriesState(_ context.Context, key model.SeriesKey) (*model.SeriesState, error) {
	st, ok := f.states[keyStr(key)]
	if !ok {
		return nil, nil
	}
	cp := *st
	return &cp, nil
}

func (f *fakeStore) ListSeriesStates(_ context.Context, scopeType string) ([]*model.SeriesState, error) {
	var out []*model.SeriesState
	for _, st := range f.states {
		if scopeType == "" || st.ScopeType == scopeType {
			cp := *st
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeStore) ListReadySeriesStates(_ context.Context) ([]*model.SeriesState, error) {
	var out []*model.SeriesState
	for _, st := range f.states {
		if st.State == model.StateReady {
			cp := *st
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeStore) InsertAnomalyEvent(_ context.Context, e *model.AnomalyEvent) error {
	if e.ID == "" {
		f.nextID++
		e.ID = "evt-" + itoa(f.nextID)
	}
	cp := *e
	f.events = append(f.events, &cp)
	return nil
}

func (f *fakeStore) GetOpenAnomalyEvent(_ context.Context, key model.SeriesKey, detector string) (*model.AnomalyEvent, error) {
	for i := len(f.events) - 1; i >= 0; i-- {
		e := f.events[i]
		if e.SeriesKey == key && e.EndedAt == nil && (detector == "" || e.Detector == detector) {
			cp := *e
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) UpdateAnomalyEvent(_ context.Context, e *model.AnomalyEvent) error {
	for _, stored := range f.events {
		if stored.ID == e.ID {
			cp := *e
			*stored = cp
			return nil
		}
	}
	return nil
}

func (f *fakeStore) CloseAnomalyEvent(_ context.Context, id string, endedAt int64) error {
	for _, e := range f.events {
		if e.ID == id && e.EndedAt == nil {
			ts := endedAt
			e.EndedAt = &ts
		}
	}
	return nil
}

func (f *fakeStore) ListAnomalyEvents(_ context.Context, filt model.AnomalyEventFilter) ([]*model.AnomalyEvent, error) {
	var out []*model.AnomalyEvent
	for _, e := range f.events {
		if filt.ScopeType != "" && e.ScopeType != filt.ScopeType {
			continue
		}
		if filt.ScopeID != "" && e.ScopeID != filt.ScopeID {
			continue
		}
		if filt.Metric != "" && e.Metric != filt.Metric {
			continue
		}
		if filt.Tier != "" && e.Tier != filt.Tier {
			continue
		}
		if filt.Active != nil {
			open := e.EndedAt == nil
			if *filt.Active != open {
				continue
			}
		}
		cp := *e
		out = append(out, &cp)
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
