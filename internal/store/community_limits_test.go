// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/uid"
)

// Acceptance tests of 051-community-label-limits at the storage boundary:
// the rights fixed at the first start (FR-003), the limited state and the
// counts that feed the edition report (FR-017), the
// inactive certificate monitors, and the copy to PostgreSQL (FR-019).

func applyMigration44(t *testing.T, db *DB, direction string) {
	t.Helper()
	matches, err := fs.Glob(migrationFS, "migrations/"+db.dialect.String()+"/44_*."+direction+".sql")
	require.NoError(t, err)
	require.Len(t, matches, 1, "migration 44 (%s) must exist for %s", direction, db.dialect.String())
	sqlText, err := fs.ReadFile(migrationFS, matches[0])
	require.NoError(t, err)
	_, err = db.ReadDB().ExecContext(context.Background(), string(sqlText))
	require.NoError(t, err, "migration 44 %s", direction)
}

func clLabelEndpoint(t *testing.T, eps *EndpointStore, agentID, container, key, target string) string {
	t.Helper()
	id, err := eps.UpsertEndpoint(context.Background(), &endpoint.Endpoint{
		AgentID: agentID, ContainerName: container, LabelKey: key, ExternalID: "ext-" + container,
		EndpointType: endpoint.TypeTCP, Target: target, Config: endpoint.DefaultConfig(), Source: endpoint.SourceLabel,
	})
	require.NoError(t, err)
	return id
}

func clStandaloneEndpoint(t *testing.T, eps *EndpointStore, name, target string) string {
	t.Helper()
	id, err := eps.InsertStandaloneEndpoint(context.Background(), &endpoint.Endpoint{
		Name: name, EndpointType: endpoint.TypeTCP, Target: target, Config: endpoint.DefaultConfig(), Source: endpoint.SourceStandalone,
	})
	require.NoError(t, err)
	return id
}

func clCertMonitor(t *testing.T, certs *CertificateStore, host string, port int, source certificate.CertSource, externalID string, endpointID *string) string {
	t.Helper()
	id, err := certs.CreateMonitor(context.Background(), &certificate.CertMonitor{
		Hostname: host, Port: port, Source: source, ExternalID: externalID, EndpointID: endpointID,
		Status: certificate.StatusUnknown, CheckIntervalSeconds: 43200, WarningThresholds: certificate.DefaultWarningThresholds(),
	})
	require.NoError(t, err)
	return id
}

// FR-003, US1 scenarios 1, 3 and 5, SC-001, edge case "nouvelle installation": every
// endpoint and every declared certificate present at the first start of the version
// keeps its right, inactive ones and remote ones included; what appears afterwards
// has none.
func TestMigration44_GrandfathersWhatExistsAtFirstStart(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	eps := NewEndpointStore(db)
	certs := NewCertificateStore(db)
	seedAgent(t, db, "agent-nas")

	running := clLabelEndpoint(t, eps, uid.LocalAgent, "web", "maintenant.endpoint.http", "127.0.0.1:8080")
	stopped := clLabelEndpoint(t, eps, uid.LocalAgent, "nightly-job", "maintenant.endpoint.tcp", "127.0.0.1:5432")
	gone := clLabelEndpoint(t, eps, uid.LocalAgent, "old-job", "maintenant.endpoint.tcp", "127.0.0.1:6379")
	require.NoError(t, eps.DeactivateEndpoint(ctx, gone))
	remote := clLabelEndpoint(t, eps, "agent-nas", "nas-web", "maintenant.endpoint.http", "127.0.0.1:9000")
	manual := clStandaloneEndpoint(t, eps, "router", "192.0.2.1:22")

	standaloneCert := clCertMonitor(t, certs, "example.com", 443, certificate.SourceStandalone, "", nil)
	labelCert := clCertMonitor(t, certs, "shop.example.com", 443, certificate.SourceLabel, "ext-web", nil)
	autoCert := clCertMonitor(t, certs, "web.internal", 8443, certificate.SourceAuto, "", &running)

	applyMigration44(t, db, "down")
	applyMigration44(t, db, "up")

	listed, err := eps.ListEndpoints(ctx, endpoint.ListEndpointsOpts{IncludeInactive: true})
	require.NoError(t, err)
	rights := map[string]bool{}
	for _, ep := range listed {
		rights[ep.ID] = ep.Grandfathered
		assert.False(t, ep.Limited, "the upgrade itself limits nothing (%s)", ep.ContainerName)
	}
	assert.Equal(t, map[string]bool{running: true, stopped: true, gone: true, remote: true, manual: true}, rights)

	for id, want := range map[string]bool{standaloneCert: true, labelCert: true, autoCert: false} {
		m, err := certs.GetMonitorByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, m)
		assert.Equal(t, want, m.Grandfathered, "%s monitor %s:%d", m.Source, m.Hostname, m.Port)
	}

	late := clLabelEndpoint(t, eps, uid.LocalAgent, "new-app", "maintenant.endpoint.http", "127.0.0.1:3000")
	lateEp, err := eps.GetEndpointByID(ctx, late)
	require.NoError(t, err)
	assert.False(t, lateEp.Grandfathered, "an endpoint that appears after the first start has no right")

	lateCert := clCertMonitor(t, certs, "new.example.com", 443, certificate.SourceLabel, "ext-new-app", nil)
	lateMon, err := certs.GetMonitorByID(ctx, lateCert)
	require.NoError(t, err)
	assert.False(t, lateMon.Grandfathered, "a certificate declared after the first start has no right")
}

// Edge case "une instance qui passe directement d'une version ancienne (1.2, 1.4)":
// a database that predates the UUID conversion keeps the rights of everything it
// holds once the new version has migrated and converted it.
func TestMigration44_LegacyDatabaseKeepsItsRightsThroughTheConversion(t *testing.T) {
	requireSQLite(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=ON")
	require.NoError(t, err)
	raw.SetMaxOpenConns(1)
	entries, err := fs.ReadDir(migrationFS, "migrations/sqlite")
	require.NoError(t, err)
	type mig struct {
		n   int
		sql string
	}
	var ups []mig
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		n, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		require.NoError(t, err)
		if n > 21 {
			continue
		}
		data, err := fs.ReadFile(migrationFS, "migrations/sqlite/"+e.Name())
		require.NoError(t, err)
		ups = append(ups, mig{n, string(data)})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].n < ups[j].n })
	for _, m := range ups {
		_, err := raw.Exec(m.sql)
		require.NoError(t, err, "apply migration %d", m.n)
	}
	for _, q := range []string{
		`CREATE TABLE schema_migrations (version uint64, dirty bool)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (21, 0)`,
		`INSERT INTO endpoints (container_name, label_key, external_id, endpoint_type, target, first_seen_at, last_seen_at) VALUES ('web','maintenant.endpoint.http','abc123','http','http://web:8080',100,100)`,
		`INSERT INTO endpoints (container_name, label_key, external_id, endpoint_type, target, first_seen_at, last_seen_at) VALUES ('api','maintenant.endpoint.tcp','def456','tcp','api:9000',200,200)`,
		`INSERT INTO cert_monitors (hostname, port, source, status, created_at) VALUES ('example.com',443,'standalone','valid',100)`,
	} {
		_, err := raw.Exec(q)
		require.NoError(t, err, q)
	}
	require.NoError(t, raw.Close())

	logger := testLogger()
	db, err := Open(path, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, Migrate(ctx, db, logger))
	wctx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	db.StartWriter(wctx)

	listed, err := NewEndpointStore(db).ListEndpoints(ctx, endpoint.ListEndpointsOpts{IncludeInactive: true})
	require.NoError(t, err)
	require.Len(t, listed, 2)
	for _, ep := range listed {
		assert.True(t, ep.Grandfathered, "legacy endpoint %s keeps its right through the conversion", ep.ContainerName)
	}

	m, err := NewCertificateStore(db).GetMonitorByID(ctx, uid.CertMonitor(uid.LocalAgent, "example.com", 443, ""))
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.True(t, m.Grandfathered, "legacy standalone certificate keeps its right through the conversion")
}

// FR-006, FR-017: the limited state is stored per endpoint, and the counts read
// active rows only.
func TestEndpointStore_LimitedStateAndCounts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	eps := NewEndpointStore(db)

	var labels []string
	for i := range 5 {
		labels = append(labels, clLabelEndpoint(t, eps, uid.LocalAgent, fmt.Sprintf("svc-%d", i), "maintenant.endpoint.tcp", fmt.Sprintf("127.0.0.1:%d", 7000+i)))
	}
	retired := clLabelEndpoint(t, eps, uid.LocalAgent, "retired", "maintenant.endpoint.tcp", "127.0.0.1:7999")
	require.NoError(t, eps.DeactivateEndpoint(ctx, retired))
	manualA := clStandaloneEndpoint(t, eps, "router", "192.0.2.1:22")
	manualB := clStandaloneEndpoint(t, eps, "nas", "192.0.2.2:445")

	for _, id := range []string{labels[1], labels[3], labels[4], retired, manualB} {
		require.NoError(t, eps.SetEndpointLimited(ctx, id, true))
	}

	for id, want := range map[string]bool{labels[0]: false, labels[1]: true, labels[4]: true, manualA: false, manualB: true} {
		ep, err := eps.GetEndpointByID(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, want, ep.Limited, ep.ContainerName+ep.Name)
	}
	listed, err := eps.ListEndpoints(ctx, endpoint.ListEndpointsOpts{})
	require.NoError(t, err)
	limitedListed := 0
	for _, ep := range listed {
		if ep.Limited {
			limitedListed++
		}
	}
	assert.Equal(t, 4, limitedListed, "the list carries the state of every active endpoint")

	count := func(f func(context.Context) (int, error)) int {
		t.Helper()
		n, err := f(ctx)
		require.NoError(t, err)
		return n
	}
	assert.Equal(t, 7, count(func(ctx context.Context) (int, error) { return eps.CountActiveEndpoints(ctx, "") }))
	assert.Equal(t, 4, count(eps.CountLimitedEndpoints), "an inactive limited endpoint is not counted")

	require.NoError(t, eps.SetEndpointLimited(ctx, labels[3], false))
	assert.Equal(t, 3, count(eps.CountLimitedEndpoints))
	ep, err := eps.GetEndpointByID(ctx, labels[3])
	require.NoError(t, err)
	assert.False(t, ep.Limited)
}

// Contract "Les certificats inactifs n'apparaissent pas", FR-006 (a limited monitor
// is never scanned), FR-010 (an inactive monitor is purged after 30 days).
func TestCertificateStore_InactiveAndLimitedMonitors(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	eps := NewEndpointStore(db)
	certs := NewCertificateStore(db)

	ep := clLabelEndpoint(t, eps, uid.LocalAgent, "web", "maintenant.endpoint.http", "127.0.0.1:8443")
	standalone := clCertMonitor(t, certs, "example.com", 443, certificate.SourceStandalone, "", nil)
	kept := clCertMonitor(t, certs, "a.example.com", 443, certificate.SourceLabel, "ext-a", nil)
	waiting := clCertMonitor(t, certs, "b.example.com", 443, certificate.SourceLabel, "ext-b", nil)
	recreated := clCertMonitor(t, certs, "c.example.com", 443, certificate.SourceLabel, "ext-c", nil)
	old := clCertMonitor(t, certs, "d.example.com", 443, certificate.SourceLabel, "ext-d", nil)
	auto := clCertMonitor(t, certs, "127.0.0.1", 8443, certificate.SourceAuto, "", &ep)

	require.NoError(t, certs.DeactivateMonitor(ctx, recreated))
	require.NoError(t, certs.DeactivateMonitor(ctx, old))
	require.NoError(t, certs.SetMonitorLimited(ctx, waiting, true))
	require.NoError(t, certs.SetMonitorLimited(ctx, old, true))

	ids := func(ms []*certificate.CertMonitor) []string {
		out := []string{}
		for _, m := range ms {
			out = append(out, m.ID)
		}
		sort.Strings(out)
		return out
	}
	sorted := func(in ...string) []string { sort.Strings(in); return in }

	listed, err := certs.ListMonitors(ctx, certificate.ListCertificatesOpts{})
	require.NoError(t, err)
	assert.Equal(t, sorted(standalone, kept, waiting, auto), ids(listed), "inactive monitors leave the list")
	all, err := certs.ListMonitors(ctx, certificate.ListCertificatesOpts{IncludeInactive: true})
	require.NoError(t, err)
	assert.Equal(t, sorted(standalone, kept, waiting, recreated, old, auto), ids(all))

	m, err := certs.GetMonitorByID(ctx, recreated)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.InactiveSince, "a deactivated monitor records when it became inactive")
	assert.WithinDuration(t, time.Now(), *m.InactiveSince, time.Minute)
	w, err := certs.GetMonitorByID(ctx, waiting)
	require.NoError(t, err)
	assert.True(t, w.Limited)

	due, err := certs.ListDueScheduledMonitors(ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, sorted(standalone, kept), ids(due), "neither a limited nor an inactive monitor is scanned")

	n, err := certs.CountActiveMonitors(ctx, certificate.SourceStandalone, certificate.SourceLabel)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = certs.CountActiveMonitors(ctx)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	n, err = certs.CountActiveMonitors(ctx, certificate.SourceAuto)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = certs.CountLimitedMonitors(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "an inactive limited monitor is not counted")

	_, err = db.Writer().Exec(ctx, `UPDATE cert_monitors SET inactive_since=? WHERE id=?`,
		time.Now().Add(-31*24*time.Hour).Unix(), old)
	require.NoError(t, err)
	deleted, err := certs.DeleteInactiveMonitorsBefore(ctx, time.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	gone, err := certs.GetMonitorByID(ctx, old)
	require.NoError(t, err)
	assert.Nil(t, gone, "purged with its right after 30 days")
	still, err := certs.GetMonitorByID(ctx, recreated)
	require.NoError(t, err)
	assert.NotNil(t, still, "inactive for less than 30 days, it keeps its row")
}

// FR-010: the retention pass purges a certificate monitor inactive for 30 days, like
// an endpoint, and leaves the recent ones.
func TestRetention_PurgesCertificateMonitorsInactiveFor30Days(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	certs := NewCertificateStore(db)

	stale := clCertMonitor(t, certs, "stale.example.com", 443, certificate.SourceLabel, "ext-stale", nil)
	recent := clCertMonitor(t, certs, "recent.example.com", 443, certificate.SourceLabel, "ext-recent", nil)
	active := clCertMonitor(t, certs, "active.example.com", 443, certificate.SourceLabel, "ext-active", nil)
	require.NoError(t, certs.DeactivateMonitor(ctx, stale))
	require.NoError(t, certs.DeactivateMonitor(ctx, recent))
	_, err := db.Writer().Exec(ctx, `UPDATE cert_monitors SET inactive_since=? WHERE id=?`,
		time.Now().Add(-31*24*time.Hour).Unix(), stale)
	require.NoError(t, err)

	rctx, cancel := context.WithCancel(ctx)
	stopped := StartRetentionCleanupWithOpts(rctx, NewContainerStore(db), db, testLogger(), RetentionOpts{CertificateStore: certs})
	t.Cleanup(func() { cancel(); <-stopped })

	require.Eventually(t, func() bool {
		m, err := certs.GetMonitorByID(ctx, stale)
		return err == nil && m == nil
	}, 10*time.Second, 50*time.Millisecond, "a monitor inactive for 31 days is purged by the retention pass")
	for _, id := range []string{recent, active} {
		m, err := certs.GetMonitorByID(ctx, id)
		require.NoError(t, err)
		assert.NotNil(t, m)
	}
}

// FR-019: the copy to PostgreSQL carries the rights, the limited state and the
// inactive mark, so the copied instance monitors exactly the same elements.
func TestCopy_CarriesRightsAndLimits(t *testing.T) {
	dst := openEmptyTarget(t)
	src, _ := seededSource(t)
	ctx := context.Background()

	applyMigration44(t, src, "down")
	applyMigration44(t, src, "up")

	var seeded string
	require.NoError(t, src.ReadDB().QueryRowContext(ctx, `SELECT id FROM endpoints`).Scan(&seeded))

	eps := NewEndpointStore(src)
	certs := NewCertificateStore(src)
	fresh := clLabelEndpoint(t, eps, uid.LocalAgent, "late", "maintenant.endpoint.tcp", "127.0.0.1:7100")
	require.NoError(t, eps.SetEndpointLimited(ctx, fresh, true))
	limitedCert := clCertMonitor(t, certs, "late.example.com", 443, certificate.SourceLabel, "ext-late", nil)
	require.NoError(t, certs.SetMonitorLimited(ctx, limitedCert, true))
	inactiveCert := clCertMonitor(t, certs, "gone.example.com", 443, certificate.SourceLabel, "ext-gone", nil)
	require.NoError(t, certs.DeactivateMonitor(ctx, inactiveCert))

	var out bytes.Buffer
	report, err := Copy(ctx, src.ReadDB(), dst, &out, func(Plan) bool { return true })
	require.NoError(t, err)
	require.True(t, report.Verified)

	endpointState := func(id string) [2]int {
		t.Helper()
		var gf, lim int
		require.NoError(t, dst.QueryRow(`SELECT grandfathered, limited FROM endpoints WHERE id=$1`, id).Scan(&gf, &lim))
		return [2]int{gf, lim}
	}
	assert.Equal(t, [2]int{1, 0}, endpointState(seeded), "a grandfathered endpoint keeps its right in the copy")
	assert.Equal(t, [2]int{0, 1}, endpointState(fresh), "a limited endpoint stays limited in the copy")

	var certGF, certLim int
	require.NoError(t, dst.QueryRow(`SELECT grandfathered, limited FROM cert_monitors WHERE id=$1`, limitedCert).Scan(&certGF, &certLim))
	assert.Equal(t, [2]int{0, 1}, [2]int{certGF, certLim})
	var inactiveSince sql.NullInt64
	require.NoError(t, dst.QueryRow(`SELECT inactive_since FROM cert_monitors WHERE id=$1`, inactiveCert).Scan(&inactiveSince))
	assert.True(t, inactiveSince.Valid, "an inactive monitor stays inactive in the copy")
	var seededCertGF int
	require.NoError(t, dst.QueryRow(`SELECT grandfathered FROM cert_monitors WHERE hostname='example.com' AND source='standalone'`).Scan(&seededCertGF))
	assert.Equal(t, 1, seededCertGF, "a certificate's right travels with it")
}
