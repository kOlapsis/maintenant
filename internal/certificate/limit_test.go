// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func TestMain(m *testing.M) {
	extension.Register(tiers.Policy{}, nil)
	os.Exit(m.Run())
}

type limitBench struct {
	t   *testing.T
	ctx context.Context
	db  *store.DB
	st  *store.CertificateStore
	svc *certificate.Service

	mu      sync.Mutex
	changed map[string]bool
	created map[string]bool
	targets []string
}

func pinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	prev := extension.CurrentEdition
	extension.CurrentEdition = func() extension.Edition { return e }
	t.Cleanup(func() { extension.CurrentEdition = prev })
}

func newLimitBench(t *testing.T) *limitBench {
	t.Helper()
	db := storetest.Open(t, nil)
	b := &limitBench{t: t, ctx: context.Background(), db: db, st: store.NewCertificateStore(db)}
	b.restart()
	return b
}

func (b *limitBench) restart() {
	b.changed, b.created = map[string]bool{}, map[string]bool{}
	b.svc = certificate.NewService(certificate.Deps{
		Store:  b.st,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		HTTPSTargets: func(context.Context) ([]string, error) {
			return b.targets, nil
		},
		EventCallback: func(eventType string, data interface{}) {
			m, _ := data.(map[string]interface{})
			id, _ := m["monitor_id"].(string)
			limited, ok := m["limited"].(bool)
			b.mu.Lock()
			defer b.mu.Unlock()
			switch eventType {
			case event.CertificateLimitChanged:
				b.changed[id] = limited
			case event.CertificateCreated:
				require.True(b.t, ok, "certificate.created carries limited: %v", m)
				b.created[id] = limited
			}
		},
	})
}

// events returns the limit changes and creations announced since the last call.
func (b *limitBench) events() (changed, created map[string]bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed, created = b.changed, b.created
	b.changed, b.created = map[string]bool{}, map[string]bool{}
	return changed, created
}

func (b *limitBench) declare(ext string, hosts ...string) {
	b.svc.SyncFromLabels(b.ctx, ext, map[string]string{"maintenant.tls.certificates": strings.Join(hosts, ",")})
}

func (b *limitBench) exec(query string, args ...any) {
	b.t.Helper()
	_, err := b.db.Writer().Exec(b.ctx, query, args...)
	require.NoError(b.t, err, query)
}

func (b *limitBench) monitor(host string) *certificate.CertMonitor {
	b.t.Helper()
	all, err := b.st.ListMonitors(b.ctx, certificate.ListCertificatesOpts{IncludeInactive: true})
	require.NoError(b.t, err)
	for _, m := range all {
		if m.Hostname == host {
			return m
		}
	}
	b.t.Fatalf("no monitor for %s", host)
	return nil
}

// split returns the listed non-auto monitors, monitored then limited, by hostname.
func (b *limitBench) split() (monitored, limited []string) {
	b.t.Helper()
	monitored, limited = []string{}, []string{}
	all, err := b.st.ListMonitors(b.ctx, certificate.ListCertificatesOpts{})
	require.NoError(b.t, err)
	for _, m := range all {
		switch {
		case m.Source == certificate.SourceAuto:
		case m.Limited:
			limited = append(limited, m.Hostname)
		default:
			monitored = append(monitored, m.Hostname)
		}
	}
	sort.Strings(monitored)
	sort.Strings(limited)
	return monitored, limited
}

func (b *limitBench) ageCreation(hosts ...string) {
	base := time.Now().Add(-48 * time.Hour)
	for i, h := range hosts {
		b.exec(`UPDATE cert_monitors SET created_at=? WHERE id=?`, base.Add(time.Duration(i)*time.Minute).Unix(), b.monitor(h).ID)
	}
}

func (b *limitBench) addAuto(host string) {
	b.t.Helper()
	_, err := b.st.CreateMonitor(b.ctx, &certificate.CertMonitor{
		Hostname: host, Port: 443, Source: certificate.SourceAuto, Status: certificate.StatusUnknown,
		CheckIntervalSeconds: 43200, WarningThresholds: certificate.DefaultWarningThresholds(),
	})
	require.NoError(b.t, err)
}

func hosts(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s-%02d.example.com", prefix, i))
	}
	return out
}

func TestApplyLimit_UnderTheCapNothingChanges(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	for _, h := range hosts("auto", 1, 4) {
		b.addAuto(h)
	}
	b.declare("ext-a", hosts("cert", 1, 5)...)

	monitored, limited := b.split()
	assert.Equal(t, hosts("cert", 1, 5), monitored)
	assert.Empty(t, limited)
	changed, created := b.events()
	assert.Empty(t, changed, "under the cap no limit change is announced")
	assert.Len(t, created, 5)
	for id, l := range created {
		assert.False(t, l, id)
	}
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	changed, _ = b.events()
	assert.Empty(t, changed)

	b.declare("ext-b", "cert-06.example.com")
	_, limited = b.split()
	assert.Equal(t, []string{"cert-06.example.com"}, limited, "a sixth label certificate is limited, auto ones never count")
	for _, h := range hosts("auto", 1, 4) {
		assert.False(t, b.monitor(h).Limited, h)
	}
	_, created = b.events()
	assert.Equal(t, map[string]bool{b.monitor("cert-06.example.com").ID: true}, created)
}

func TestApplyLimit_LimitedMonitorIsNeitherScheduledNorChecked(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", hosts("cert", 1, 7)...)
	_, limited := b.split()
	require.Equal(t, hosts("cert", 6, 7), limited)

	due, err := b.st.ListDueScheduledMonitors(b.ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	var scheduled []string
	for _, m := range due {
		scheduled = append(scheduled, m.Hostname)
	}
	sort.Strings(scheduled)
	assert.Equal(t, hosts("cert", 1, 5), scheduled)

	_, err = b.svc.CheckNow(b.ctx, b.monitor("cert-06.example.com").ID)
	assert.ErrorIs(t, err, certificate.ErrLimited)
}

func TestApplyLimit_FreedSlotGoesToTheOldestWaiting(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	manual, _, err := b.svc.CreateStandalone(b.ctx, certificate.CreateCertificateInput{Hostname: "127.0.0.1", Port: 1, CheckIntervalSeconds: 3600})
	require.NoError(t, err)
	b.declare("ext-a", hosts("a", 1, 4)...)
	b.declare("ext-b", hosts("b", 1, 2)...)
	b.ageCreation(append([]string{"127.0.0.1"}, append(hosts("a", 1, 4), hosts("b", 1, 2)...)...)...)
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited := b.split()
	require.Equal(t, hosts("b", 1, 2), limited)
	b.events()

	require.NoError(t, b.svc.DeleteMonitor(b.ctx, manual.ID))
	_, limited = b.split()
	assert.Equal(t, []string{"b-02.example.com"}, limited)
	changed, _ := b.events()
	assert.Equal(t, map[string]bool{b.monitor("b-01.example.com").ID: false}, changed)
}

func TestApplyLimit_VanishedMonitorsFreeTheirSlotsAtOnce(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", hosts("a", 1, 5)...)
	b.declare("ext-b", "b-01.example.com")
	_, limited := b.split()
	require.Equal(t, []string{"b-01.example.com"}, limited)

	b.svc.HandleContainerDestroy(b.ctx, "ext-a")
	monitored, limited := b.split()
	assert.Equal(t, []string{"b-01.example.com"}, monitored)
	assert.Empty(t, limited)
}

func TestApplyLimit_EditionChangesAndRestart(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	all := hosts("cert", 1, 8)
	b.declare("ext-a", all...)
	order := []string{all[6], all[1], all[4], all[0], all[7], all[2], all[5], all[3]}
	b.ageCreation(order...)
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	monitored, limited := b.split()
	wantMonitored := append([]string(nil), order[:5]...)
	sort.Strings(wantMonitored)
	wantLimited := append([]string(nil), order[5:]...)
	sort.Strings(wantLimited)
	require.Equal(t, wantMonitored, monitored, "the oldest stay monitored")
	require.Equal(t, wantLimited, limited)
	b.events()

	pinEdition(t, extension.Pro)
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited = b.split()
	assert.Empty(t, limited)
	changed, _ := b.events()
	want := map[string]bool{}
	for _, h := range wantLimited {
		want[b.monitor(h).ID] = false
	}
	assert.Equal(t, want, changed, "exactly the waiting monitors are announced")

	b.exec(`UPDATE cert_monitors SET grandfathered=1 WHERE hostname IN (?, ?)`, order[7], order[6])
	pinEdition(t, extension.Community)
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	monitored, limited = b.split()
	wantMonitored = append([]string{order[6], order[7]}, order[:3]...)
	sort.Strings(wantMonitored)
	wantLimited = append([]string(nil), order[3:6]...)
	sort.Strings(wantLimited)
	assert.Equal(t, wantMonitored, monitored, "back to Community: grandfathered first, then the oldest")
	assert.Equal(t, wantLimited, limited)
	b.events()

	b.restart()
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	changed, _ = b.events()
	assert.Empty(t, changed, "a restart changes nothing")
	monitored, limited = b.split()
	assert.Equal(t, wantMonitored, monitored)
	assert.Equal(t, wantLimited, limited)
}

func TestCreateStandalone_CountsLabelAndGrandfatheredMonitors(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", hosts("cert", 1, 4)...)
	_, _, err := b.svc.CreateStandalone(b.ctx, certificate.CreateCertificateInput{Hostname: "127.0.0.1", Port: 1, CheckIntervalSeconds: 3600})
	require.NoError(t, err)
	_, _, err = b.svc.CreateStandalone(b.ctx, certificate.CreateCertificateInput{Hostname: "127.0.0.1", Port: 2, CheckIntervalSeconds: 3600})
	assert.ErrorIs(t, err, certificate.ErrLimitReached, "five label and manual monitors fill the cap")

	b.declare("ext-b", hosts("more", 1, 2)...)
	b.exec(`UPDATE cert_monitors SET grandfathered=1`)
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited := b.split()
	require.Empty(t, limited)
	_, _, err = b.svc.CreateStandalone(b.ctx, certificate.CreateCertificateInput{Hostname: "127.0.0.1", Port: 2, CheckIntervalSeconds: 3600})
	assert.ErrorIs(t, err, certificate.ErrLimitReached, "past the cap by grandfathering, manual creation is refused")

	_, limited = b.split()
	assert.Empty(t, limited, "a refused creation is never stored as limited")
}

func TestLabelMonitor_RecreationKeepsIdentityAndHistory(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", "cert-01.example.com", "cert-02.example.com")
	b.exec(`UPDATE cert_monitors SET grandfathered=1, created_at=?`, time.Now().Add(-72*time.Hour).Unix())
	before := b.monitor("cert-01.example.com")
	_, err := b.st.InsertCheckResult(b.ctx, &certificate.CertCheckResult{MonitorID: before.ID, CheckedAt: time.Now(), ErrorMessage: "refused"})
	require.NoError(t, err)
	b.events()

	b.svc.HandleContainerDestroy(b.ctx, "ext-a")
	monitored, _ := b.split()
	assert.Empty(t, monitored, "the monitors of a destroyed container leave the list")

	b.declare("ext-a-2", "cert-01.example.com", "cert-02.example.com")
	after := b.monitor("cert-01.example.com")
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.CreatedAt.Unix(), after.CreatedAt.Unix())
	assert.True(t, after.Grandfathered)
	assert.Nil(t, after.InactiveSince)
	assert.Equal(t, "ext-a-2", after.ExternalID)
	_, total, err := b.st.ListCheckResults(b.ctx, after.ID, certificate.ListChecksOpts{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "history survives the recreation")
	_, created := b.events()
	assert.Equal(t, map[string]bool{after.ID: false, b.monitor("cert-02.example.com").ID: false}, created, "readoption is announced")

	b.declare("ext-a-3", "cert-01.example.com", "cert-02.example.com")
	b.svc.HandleContainerDestroy(b.ctx, "ext-a-2")
	monitored, _ = b.split()
	assert.Equal(t, []string{"cert-01.example.com", "cert-02.example.com"}, monitored, "start before destroy keeps the monitors")
	assert.Equal(t, before.ID, b.monitor("cert-01.example.com").ID)

	b.declare("ext-a-3", "cert-01.example.com")
	monitored, _ = b.split()
	assert.Equal(t, []string{"cert-01.example.com"}, monitored, "a target removed from the label is deleted")
	all, err := b.st.ListMonitors(b.ctx, certificate.ListCertificatesOpts{IncludeInactive: true})
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestLabelMonitor_PurgedAfter30DaysComesBackNew(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", "cert-01.example.com")
	b.exec(`UPDATE cert_monitors SET grandfathered=1, created_at=?`, time.Now().Add(-90*24*time.Hour).Unix())
	b.svc.HandleContainerDestroy(b.ctx, "ext-a")
	b.exec(`UPDATE cert_monitors SET inactive_since=?`, time.Now().Add(-31*24*time.Hour).Unix())

	n, err := b.st.DeleteInactiveMonitorsBefore(b.ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	b.declare("ext-a-2", "cert-01.example.com")
	m := b.monitor("cert-01.example.com")
	assert.False(t, m.Grandfathered, "the right disappears with the purge")
	assert.WithinDuration(t, time.Now(), m.CreatedAt, time.Minute)
}

func TestHandleAgentEvent_IgnoresResultsOfALimitedMonitor(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	const agentID = "00000000-0000-0000-0000-000000000052"
	require.NoError(t, store.NewAgentStore(b.db).Insert(b.ctx, &agent.Agent{
		AgentID: agentID, Hostname: "nas", Label: "nas", OSArch: "linux/amd64",
		AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
	}))
	b.declare("ext-a", hosts("cert", 1, 5)...)

	ev := &agentpb.CertificateInfo{
		Host: "nas.example.com", Port: 443, SubjectCn: "nas.example.com",
		NotBefore: timestamppb.New(time.Now().Add(-time.Hour)), NotAfter: timestamppb.New(time.Now().Add(90 * 24 * time.Hour)),
	}
	require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentID, ev))
	m := b.monitor("nas.example.com")
	assert.True(t, m.Limited, "a pushed monitor past the cap is limited")
	_, created := b.events()
	assert.Equal(t, true, created[m.ID])

	require.NoError(t, b.svc.HandleAgentEvent(b.ctx, agentID, ev))
	_, total, err := b.st.ListCheckResults(b.ctx, m.ID, certificate.ListChecksOpts{Limit: 10})
	require.NoError(t, err)
	assert.Zero(t, total, "a result pushed for a limited monitor is ignored")
	assert.Equal(t, certificate.StatusUnknown, b.monitor("nas.example.com").Status)
}

func TestApplyLimit_CertificateOfAMonitoredHTTPSEndpointIsNotCounted(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", hosts("a", 1, 5)...)
	b.declare("ext-b", "b-01.example.com")
	_, limited := b.split()
	require.Equal(t, []string{"b-01.example.com"}, limited)
	b.events()

	b.targets = []string{"https://b-01.example.com/health"}
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited = b.split()
	assert.Empty(t, limited)
	changed, _ := b.events()
	assert.Equal(t, map[string]bool{b.monitor("b-01.example.com").ID: false}, changed)
	used, unmonitored, err := b.svc.LimitUsage(b.ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, used)
	assert.Zero(t, unmonitored)

	b.targets = []string{"https://b-01.example.com:8443"}
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited = b.split()
	assert.Equal(t, []string{"b-01.example.com"}, limited, "another port is another certificate")

	b.targets = nil
	require.NoError(t, b.svc.ApplyLimit(b.ctx))
	_, limited = b.split()
	assert.Equal(t, []string{"b-01.example.com"}, limited)
}
