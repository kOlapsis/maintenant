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

package outbound

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	mu    sync.Mutex
	items map[string]*OutboundHeartbeat
	order []string
}

func newFakeStore(items ...*OutboundHeartbeat) *fakeStore {
	f := &fakeStore{items: map[string]*OutboundHeartbeat{}}
	for _, o := range items {
		f.items[o.ID] = o
		f.order = append(f.order, o.ID)
	}
	return f
}

func (f *fakeStore) List(context.Context) ([]*OutboundHeartbeat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*OutboundHeartbeat
	for _, id := range f.order {
		if o, ok := f.items[id]; ok {
			cp := *o
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, id string) (*OutboundHeartbeat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.items[id]
	if !ok {
		return nil, nil
	}
	cp := *o
	return &cp, nil
}

func (f *fakeStore) Create(_ context.Context, o *OutboundHeartbeat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *o
	f.items[o.ID] = &cp
	f.order = append(f.order, o.ID)
	return nil
}

func (f *fakeStore) Update(_ context.Context, o *OutboundHeartbeat) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[o.ID]; !ok {
		return false, nil
	}
	cp := *o
	f.items[o.ID] = &cp
	return true, nil
}

func (f *fakeStore) Delete(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.items[id]
	delete(f.items, id)
	return ok, nil
}

func (f *fakeStore) RecordSend(_ context.Context, id string, r SendResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.items[id]
	if !ok {
		return errors.New("not found")
	}
	at := r.SentAt
	o.LastSentAt = &at
	o.LastStatusCode = r.StatusCode
	o.LastError = r.Error
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestDue(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name string
		o    OutboundHeartbeat
		want bool
	}{
		{"never sent", OutboundHeartbeat{Enabled: true, IntervalSeconds: 60}, true},
		{"disabled", OutboundHeartbeat{Enabled: false, IntervalSeconds: 60}, false},
		{"sent recently", OutboundHeartbeat{Enabled: true, IntervalSeconds: 60, LastSentAt: ptrTime(now.Add(-30 * time.Second))}, false},
		{"interval just elapsed", OutboundHeartbeat{Enabled: true, IntervalSeconds: 60, LastSentAt: ptrTime(now.Add(-60 * time.Second))}, true},
		{"overdue", OutboundHeartbeat{Enabled: true, IntervalSeconds: 60, LastSentAt: ptrTime(now.Add(-time.Hour))}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.o.due(now))
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		ok   bool
	}{
		{"valid https", Input{Name: "a", URL: "https://mnt.example/ping/x", IntervalSeconds: 60}, true},
		{"http rejected", Input{Name: "a", URL: "http://mnt.example/ping/x", IntervalSeconds: 30}, false},
		{"max interval", Input{Name: "a", URL: "https://h", IntervalSeconds: 86400}, true},
		{"blank name", Input{Name: "  ", URL: "https://h", IntervalSeconds: 60}, false},
		{"no scheme", Input{Name: "a", URL: "mnt.example/ping", IntervalSeconds: 60}, false},
		{"ftp scheme", Input{Name: "a", URL: "ftp://h/x", IntervalSeconds: 60}, false},
		{"no host", Input{Name: "a", URL: "https:///x", IntervalSeconds: 60}, false},
		{"interval too short", Input{Name: "a", URL: "https://h", IntervalSeconds: 29}, false},
		{"interval too long", Input{Name: "a", URL: "https://h", IntervalSeconds: 86401}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidInput)
			}
		})
	}
}

func TestSendDue_RecordsResults(t *testing.T) {
	var userAgent atomic.Value
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()
	deadSrv := httptest.NewServer(http.NotFoundHandler())
	deadURL := deadSrv.URL
	deadSrv.Close()

	now := time.Unix(1_700_000_000, 0)
	recent := now.Add(-10 * time.Second)
	st := newFakeStore(
		&OutboundHeartbeat{ID: "ok", URL: okSrv.URL, IntervalSeconds: 60, Enabled: true},
		&OutboundHeartbeat{ID: "fail", URL: failSrv.URL, IntervalSeconds: 60, Enabled: true},
		&OutboundHeartbeat{ID: "dead", URL: deadURL, IntervalSeconds: 60, Enabled: true},
		&OutboundHeartbeat{ID: "notdue", URL: okSrv.URL, IntervalSeconds: 60, Enabled: true, LastSentAt: &recent},
		&OutboundHeartbeat{ID: "off", URL: okSrv.URL, IntervalSeconds: 60, Enabled: false},
	)
	s := NewService(Deps{Store: st, Version: "1.2.3", Client: &http.Client{Timeout: time.Second}})
	s.now = func() time.Time { return now }

	s.sendDue(context.Background())
	s.wg.Wait()

	ok, _ := st.Get(context.Background(), "ok")
	require.NotNil(t, ok.LastSentAt)
	assert.Equal(t, now, *ok.LastSentAt)
	require.NotNil(t, ok.LastStatusCode)
	assert.Equal(t, 200, *ok.LastStatusCode)
	assert.Nil(t, ok.LastError)
	assert.Equal(t, "maintenant/1.2.3 outbound-heartbeat", userAgent.Load())

	fail, _ := st.Get(context.Background(), "fail")
	require.NotNil(t, fail.LastStatusCode)
	assert.Equal(t, 500, *fail.LastStatusCode)
	require.NotNil(t, fail.LastError)
	assert.Contains(t, *fail.LastError, "500")

	dead, _ := st.Get(context.Background(), "dead")
	require.NotNil(t, dead.LastSentAt)
	assert.Nil(t, dead.LastStatusCode)
	require.NotNil(t, dead.LastError)
	assert.NotContains(t, *dead.LastError, deadURL)

	notDue, _ := st.Get(context.Background(), "notdue")
	assert.Equal(t, recent, *notDue.LastSentAt)
	assert.Nil(t, notDue.LastStatusCode)

	off, _ := st.Get(context.Background(), "off")
	assert.Nil(t, off.LastSentAt)
}

func TestSendDue_SkipsTargetInFlight(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	st := newFakeStore(&OutboundHeartbeat{ID: "slow", URL: srv.URL, IntervalSeconds: 30, Enabled: true})
	s := NewService(Deps{Store: st, Client: &http.Client{Timeout: 5 * time.Second}})

	s.sendDue(context.Background())
	require.Eventually(t, func() bool { return hits.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	s.sendDue(context.Background())
	close(release)
	s.wg.Wait()

	assert.Equal(t, int32(1), hits.Load())
	got, _ := st.Get(context.Background(), "slow")
	require.NotNil(t, got.LastStatusCode)
	assert.Equal(t, 204, *got.LastStatusCode)
}

func TestSendNow(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewService(Deps{Store: newFakeStore(), Client: srv.Client(), ValidateURL: acceptURL})
	ctx := context.Background()
	o, err := s.Create(ctx, Input{Name: "up", URL: srv.URL, IntervalSeconds: 60})
	require.NoError(t, err)
	assert.True(t, o.Enabled, "enabled defaults to true")

	got, err := s.SendNow(ctx, o.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastSentAt)
	assert.Equal(t, 200, *got.LastStatusCode)

	_, err = s.SendNow(ctx, "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdateDelete_NotFound(t *testing.T) {
	s := NewService(Deps{Store: newFakeStore(), ValidateURL: acceptURL})
	ctx := context.Background()
	_, err := s.Update(ctx, "missing", Input{Name: "a", URL: "https://h", IntervalSeconds: 60})
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, s.Delete(ctx, "missing"), ErrNotFound)
}

func acceptURL(context.Context, string) error { return nil }

func TestCreate_RejectsInternalTargets(t *testing.T) {
	s := NewService(Deps{Store: newFakeStore()})
	ctx := context.Background()
	for _, u := range []string{"https://127.0.0.1/ping/x", "https://10.0.0.5/ping/x", "https://169.254.169.254/latest", "https://localhost/ping/x"} {
		_, err := s.Create(ctx, Input{Name: "a", URL: u, IntervalSeconds: 60})
		assert.ErrorIs(t, err, ErrInvalidInput, u)
	}
}

func TestSendDue_DefaultClientRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := newFakeStore(&OutboundHeartbeat{ID: "loop", URL: srv.URL, IntervalSeconds: 60, Enabled: true})
	s := NewService(Deps{Store: st})

	s.sendDue(context.Background())
	s.wg.Wait()

	assert.Equal(t, int32(0), hits.Load())
	got, _ := st.Get(context.Background(), "loop")
	assert.Nil(t, got.LastStatusCode)
	require.NotNil(t, got.LastError)
}
