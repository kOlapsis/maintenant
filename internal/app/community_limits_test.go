// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentevent"
	"github.com/kolapsis/maintenant/internal/agentpb"
	v1 "github.com/kolapsis/maintenant/internal/api/v1"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/event"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
)

// Acceptance tests of 051-community-label-limits through the running app: label
// discovery as the runtime drives it, the real router, the SSE bus, the check
// engine, the alert engine and the status page.

const (
	clEndpointRefusal    = "The Community edition is limited to 10 endpoints, label-discovered ones included."
	clCertificateRefusal = "The Community edition is limited to 5 certificate monitors, label-declared ones included."
	clAgentID            = "00000000-0000-0000-0000-000000000051"
)

type clEnv struct {
	t      *testing.T
	a      *App
	ctx    context.Context
	dbPath string
	target string
	events chan v1.SSEEvent
	stop   func()
}

var (
	clEditionOnce sync.Once
	clEditionNow  atomic.Value
)

// clPinEdition switches the edition through an atomic: the check and alert goroutines read it while a test switches.
func clPinEdition(t *testing.T, e extension.Edition) {
	t.Helper()
	clEditionOnce.Do(func() {
		clEditionNow.Store(extension.Community)
		extension.CurrentEdition = func() extension.Edition { return clEditionNow.Load().(extension.Edition) }
	})
	prev := clEditionNow.Load()
	clEditionNow.Store(e)
	t.Cleanup(func() { clEditionNow.Store(prev) })
}

// clOpen starts the services discovery feeds, on a fresh database or on dbPath.
func clOpen(t *testing.T, dbPath string, edit func(*Config)) *clEnv {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })

	cfg, _, logger := storageEnv(t)
	if dbPath != "" {
		cfg.DBPath = dbPath
	}
	if edit != nil {
		edit(&cfg)
	}
	a, err := New(cfg, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	a.db.StartWriter(ctx)
	a.alertEngine.Start(ctx)
	events := make(chan v1.SSEEvent, 16384)
	a.broker.AddObserver(events)
	a.endpointSvc.Start(ctx)

	e := &clEnv{t: t, a: a, ctx: ctx, dbPath: cfg.DBPath, target: ln.Addr().String(), events: events}
	e.stop = sync.OnceFunc(func() {
		a.checkEngine.Stop()
		cancel()
		_ = a.db.Close()
	})
	t.Cleanup(e.stop)
	return e
}

func clTCPLabels(target string, n int) map[string]string {
	labels := map[string]string{}
	for i := range n {
		labels[fmt.Sprintf("maintenant.endpoint.%d.tcp", i)] = target
	}
	return labels
}

func clHTTPSLabels(url string) map[string]string {
	return map[string]string{
		"maintenant.endpoint.http":            url + "/",
		"maintenant.endpoint.http.tls-verify": "false",
	}
}

func clCertLabels(hosts ...string) map[string]string {
	return map[string]string{"maintenant.tls.certificates": strings.Join(hosts, ",")}
}

func clNames(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s-%02d", prefix, i))
	}
	return out
}

func clExt(container string, gen int) string {
	if gen == 0 {
		return "ext-" + container
	}
	return fmt.Sprintf("ext-%s-%d", container, gen)
}

// declare runs the discovery of a container whose labels declare n TCP endpoints.
func (e *clEnv) declare(container string, n int) {
	e.a.endpointSvc.SyncEndpoints(e.ctx, container, clExt(container, 0), clTCPLabels(e.target, n))
}

func (e *clEnv) declareAll(names ...string) {
	for _, n := range names {
		e.declare(n, 1)
	}
}

type clEndpoint struct {
	ID            string `json:"id"`
	ContainerName string `json:"container_name"`
	LabelKey      string `json:"label_key"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Source        string `json:"source"`
	Limited       bool   `json:"limited"`
}

func clKey(ep clEndpoint) string {
	if ep.Source == "standalone" {
		return "manual:" + ep.Name
	}
	if ep.LabelKey == "maintenant.endpoint.0.tcp" || ep.LabelKey == "maintenant.endpoint.http" {
		return ep.ContainerName
	}
	return ep.ContainerName + "/" + ep.LabelKey
}

var clJSON = map[string]string{"Content-Type": "application/json", "Sec-Fetch-Site": "same-origin"}

func (e *clEnv) call(method, path, body string) *httptest.ResponseRecorder {
	return serve(e.a, method, path, body, clJSON)
}

func (e *clEnv) decode(rec *httptest.ResponseRecorder, wantStatus int, into any) {
	e.t.Helper()
	require.Equal(e.t, wantStatus, rec.Code, rec.Body.String())
	require.NoError(e.t, json.Unmarshal(rec.Body.Bytes(), into), rec.Body.String())
}

func (e *clEnv) endpoints() []clEndpoint {
	e.t.Helper()
	var body struct {
		Endpoints []clEndpoint `json:"endpoints"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/endpoints", ""), http.StatusOK, &body)
	return body.Endpoints
}

func (e *clEnv) find(key string) clEndpoint {
	e.t.Helper()
	for _, ep := range e.endpoints() {
		if clKey(ep) == key {
			return ep
		}
	}
	e.t.Fatalf("endpoint %q is not listed", key)
	return clEndpoint{}
}

// split returns the listed endpoints, monitored then limited, by key.
func (e *clEnv) split() (monitored, limited []string) {
	e.t.Helper()
	monitored, limited = []string{}, []string{}
	for _, ep := range e.endpoints() {
		if ep.Limited {
			limited = append(limited, clKey(ep))
		} else {
			monitored = append(monitored, clKey(ep))
		}
	}
	sort.Strings(monitored)
	sort.Strings(limited)
	return monitored, limited
}

func (e *clEnv) limited() []string {
	_, l := e.split()
	return l
}

func (e *clEnv) checksTotal(id string) int {
	e.t.Helper()
	var body struct {
		Total int `json:"total"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/endpoints/"+id+"/checks", ""), http.StatusOK, &body)
	return body.Total
}

func (e *clEnv) createEndpoint(name string) *httptest.ResponseRecorder {
	return e.call(http.MethodPost, "/api/v1/endpoints",
		fmt.Sprintf(`{"name":%q,"target":%q,"endpoint_type":"tcp"}`, name, e.target))
}

func (e *clEnv) createCertificate(port int) *httptest.ResponseRecorder {
	return e.call(http.MethodPost, "/api/v1/certificates", fmt.Sprintf(`{"hostname":"127.0.0.1","port":%d}`, port))
}

type clCert struct {
	ID         string  `json:"id"`
	Hostname   string  `json:"hostname"`
	Port       int     `json:"port"`
	Source     string  `json:"source"`
	EndpointID *string `json:"endpoint_id"`
	Limited    bool    `json:"limited"`
}

func (e *clEnv) certificates() []clCert {
	e.t.Helper()
	var body struct {
		Certificates []clCert `json:"certificates"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/certificates", ""), http.StatusOK, &body)
	return body.Certificates
}

func (e *clEnv) cert(host string) clCert {
	e.t.Helper()
	for _, c := range e.certificates() {
		if c.Hostname == host {
			return c
		}
	}
	e.t.Fatalf("certificate %q is not listed", host)
	return clCert{}
}

// certSplit returns the declared (non-auto) certificates, monitored then limited, by hostname.
func (e *clEnv) certSplit() (monitored, limited []string) {
	e.t.Helper()
	monitored, limited = []string{}, []string{}
	for _, c := range e.certificates() {
		if c.Source == string(certificate.SourceAuto) {
			continue
		}
		if c.Limited {
			limited = append(limited, c.Hostname)
		} else {
			monitored = append(monitored, c.Hostname)
		}
	}
	sort.Strings(monitored)
	sort.Strings(limited)
	return monitored, limited
}

type clQuota struct {
	Used        int  `json:"used"`
	Limit       int  `json:"limit"`
	Unmonitored *int `json:"unmonitored"`
}

func (e *clEnv) quota(resource string) clQuota {
	e.t.Helper()
	var body struct {
		Quotas map[string]clQuota `json:"quotas"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/edition", ""), http.StatusOK, &body)
	q, ok := body.Quotas[resource]
	require.True(e.t, ok, "quota %s is reported", resource)
	return q
}

func (e *clEnv) assertQuota(resource string, used, limit, unmonitored int) {
	e.t.Helper()
	q := e.quota(resource)
	assert.Equal(e.t, used, q.Used, "%s used", resource)
	assert.Equal(e.t, limit, q.Limit, "%s limit", resource)
	if assert.NotNil(e.t, q.Unmonitored, "%s quota reports the unmonitored count", resource) {
		assert.Equal(e.t, unmonitored, *q.Unmonitored, "%s unmonitored", resource)
	}
}

func (e *clEnv) refusal(rec *httptest.ResponseRecorder) v1.ErrorDetail {
	e.t.Helper()
	var body v1.ErrorResponse
	e.decode(rec, http.StatusForbidden, &body)
	return body.Error
}

func (e *clEnv) exec(query string, args ...any) {
	e.t.Helper()
	_, err := e.a.db.Writer().Exec(e.ctx, query, args...)
	require.NoError(e.t, err, query)
}

// drain returns the events broadcast since the last call.
func (e *clEnv) drain() []v1.SSEEvent {
	var out []v1.SSEEvent
	for {
		select {
		case ev := <-e.events:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func clData(t *testing.T, ev v1.SSEEvent) map[string]any {
	t.Helper()
	raw, err := json.Marshal(ev.Data)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// clLimitFlags maps the id field of every event of type to the limited flag it carries.
func clLimitFlags(t *testing.T, events []v1.SSEEvent, eventType, idField string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, ev := range events {
		if ev.Type != eventType {
			continue
		}
		d := clData(t, ev)
		limited, ok := d["limited"].(bool)
		require.True(t, ok, "%s carries a limited flag: %v", eventType, d)
		id, _ := d[idField].(string)
		out[id] = limited
	}
	return out
}

type clTLS struct {
	url   string
	conns *atomic.Int32
}

// clTLSServer counts every connection, a handshake alone included.
func clTLSServer(t *testing.T) clTLS {
	t.Helper()
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return clTLS{url: srv.URL, conns: &conns}
}

// clRollBackTo43 turns a database of this version into one of the previous version.
func clRollBackTo43(t *testing.T, dbPath string) {
	t.Helper()
	matches, err := filepath.Glob("../store/migrations/sqlite/44_*.down.sql")
	require.NoError(t, err)
	require.Len(t, matches, 1, "migration 44 must exist, with its down file")
	down, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(string(down))
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE schema_migrations SET version = 43, dirty = 0`)
	require.NoError(t, err)
}

// clUpgraded builds a previous-version database holding nEndpoints label
// endpoints (one per container, svc-NN) and nCerts label certificates
// (cert-NN.example.com, one container), then starts this version on it in
// Community and runs the startup allocation.
func clUpgraded(t *testing.T, nEndpoints, nCerts int) *clEnv {
	t.Helper()
	clPinEdition(t, extension.Pro)
	old := clOpen(t, "", nil)
	old.declareAll(clNames("svc", 1, nEndpoints)...)
	if nCerts > 0 {
		old.a.certSvc.SyncFromLabels(old.ctx, "ext-certs", clCertLabels(clCertHosts(1, nCerts)...))
	}
	path := old.dbPath
	old.stop()
	clRollBackTo43(t, path)

	clPinEdition(t, extension.Community)
	e := clOpen(t, path, nil)
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	require.NoError(t, e.a.certSvc.ApplyLimit(e.ctx))
	return e
}

func clCertHosts(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("cert-%02d.example.com", i))
	}
	return out
}

func clRunRetention(t *testing.T, e *clEnv, gone func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(e.ctx)
	stopped := store.StartRetentionCleanupWithOpts(ctx, e.a.containerStore, e.a.db, e.a.logger, store.RetentionOpts{
		EndpointStore:    e.a.epStore,
		CertificateStore: e.a.certStore,
	})
	defer func() { cancel(); <-stopped }()
	require.Eventually(t, gone, 10*time.Second, 50*time.Millisecond, "the retention pass purges it")
}

// US2 scenarios 1, 2 and 3, FR-001, FR-006, FR-007, FR-008, SC-003, edge case
// "nouvelle installation": on a new install the first ten endpoints are
// monitored, the others are listed as not monitored, never checked, and every
// surface says so.
func TestCommunityLimit_NewInstallMonitorsTheFirstTenAndShowsTheRest(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)

	e.declareAll(clNames("svc", 1, 30)...)

	eps := e.endpoints()
	require.Len(t, eps, 30, "every discovered endpoint is listed, monitored or not")
	monitored, limited := e.split()
	assert.Equal(t, clNames("svc", 1, 10), monitored)
	assert.Equal(t, clNames("svc", 11, 30), limited)
	for _, ep := range eps {
		if ep.Limited {
			assert.Equal(t, "unknown", ep.Status, "%s is counted neither up nor down", clKey(ep))
		}
	}
	assert.Equal(t, 10, e.a.checkEngine.ActiveCount(), "only monitored endpoints are checked")

	var detail struct {
		Endpoint clEndpoint `json:"endpoint"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/endpoints/"+e.find("svc-11").ID, ""), http.StatusOK, &detail)
	assert.True(t, detail.Endpoint.Limited, "the detail carries the state")
	e.decode(e.call(http.MethodGet, "/api/v1/endpoints/"+e.find("svc-10").ID, ""), http.StatusOK, &detail)
	assert.False(t, detail.Endpoint.Limited)

	discovered := clLimitFlags(t, e.drain(), event.EndpointDiscovered, "endpoint_id")
	for _, ep := range eps {
		flag, ok := discovered[ep.ID]
		if assert.True(t, ok, "%s announced at discovery", clKey(ep)) {
			assert.Equal(t, ep.Limited, flag, "%s announced with its state", clKey(ep))
		}
	}

	e.assertQuota("endpoints", 10, 10, 20)
}

// FR-008, edge case "redémarrer l'instance ne change pas lesquels sont surveillés".
func TestCommunityLimit_RestartKeepsTheSameEndpointsMonitored(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 16)...)
	monitored, limited := e.split()
	require.Equal(t, clNames("svc", 11, 16), limited)

	path := e.dbPath
	e.stop()
	again := clOpen(t, path, nil)
	require.NoError(t, again.a.endpointSvc.ApplyLimit(again.ctx))
	names := clNames("svc", 1, 16)
	for i := len(names) - 1; i >= 0; i-- {
		again.declare(names[i], 1)
	}

	m, l := again.split()
	assert.Equal(t, monitored, m)
	assert.Equal(t, limited, l)
	assert.Equal(t, 10, again.a.checkEngine.ActiveCount())
}

// FR-006, FR-002, US2 scenarios 1 and 4: an unmonitored endpoint is never dialed,
// keeps no history, raises nothing, and no certificate is detected on it, while
// the monitored HTTPS endpoint beside it is checked and its certificate detected.
func TestCommunityLimit_UnmonitoredEndpointIsNeverCheckedNorDetected(t *testing.T) {
	monitoredTLS := clTLSServer(t)
	limitedTLS := clTLSServer(t)
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)

	e.declareAll(clNames("svc", 1, 9)...)
	e.a.endpointSvc.SyncEndpoints(e.ctx, "web-https", "ext-web-https", clHTTPSLabels(monitoredTLS.url))
	e.a.endpointSvc.SyncEndpoints(e.ctx, "zz-shop-https", "ext-zz-shop-https", clHTTPSLabels(limitedTLS.url))

	web := e.find("web-https")
	shop := e.find("zz-shop-https")
	require.False(t, web.Limited)
	require.True(t, shop.Limited)

	require.Eventually(t, func() bool {
		m, err := e.a.certStore.GetMonitorByEndpointID(e.ctx, web.ID)
		return err == nil && m != nil
	}, 15*time.Second, 50*time.Millisecond, "the monitored HTTPS endpoint is checked and its certificate detected")
	assert.Positive(t, monitoredTLS.conns.Load())

	assert.Zero(t, limitedTLS.conns.Load(), "nothing ever connects to an unmonitored endpoint")
	m, err := e.a.certStore.GetMonitorByEndpointID(e.ctx, shop.ID)
	require.NoError(t, err)
	assert.Nil(t, m, "no certificate is detected for an unmonitored endpoint")
	assert.Zero(t, e.checksTotal(shop.ID), "an unmonitored endpoint keeps no history")

	d := e.refusal(e.call(http.MethodPost, "/api/v1/endpoints/"+shop.ID+"/check", ""))
	assert.Equal(t, "QUOTA_EXCEEDED", d.Code)
	assert.Equal(t, "endpoints", d.Resource)
	assert.Zero(t, limitedTLS.conns.Load(), "an on-demand check is refused, not run")

	for _, ev := range e.drain() {
		if ev.Type == event.EndpointStatusChanged || ev.Type == event.EndpointAlert {
			assert.NotEqual(t, shop.ID, clData(t, ev)["endpoint_id"], "%s raised for an unmonitored endpoint", ev.Type)
		}
	}
}

// US2 scenario 7, US3 scenario 2, FR-004, FR-014: a manual creation counts every
// origin, and the refusal names the cap, its origins and the edition that lifts it.
func TestCommunityLimit_ManualEndpointCreationCountsEveryOrigin(t *testing.T) {
	for name, tc := range map[string]struct{ labelled, accepted int }{
		"nine label endpoints": {9, 1},
		"ten label endpoints":  {10, 0},
		"four label endpoints": {4, 6},
	} {
		t.Run(name, func(t *testing.T) {
			clPinEdition(t, extension.Community)
			e := clOpen(t, "", nil)
			e.declareAll(clNames("svc", 1, tc.labelled)...)

			for i := range tc.accepted {
				rec := e.createEndpoint(fmt.Sprintf("manual-%d", i))
				require.Equal(t, http.StatusCreated, rec.Code, "creation %d: %s", i, rec.Body.String())
			}
			d := e.refusal(e.createEndpoint("one-too-many"))
			assert.Equal(t, "QUOTA_EXCEEDED", d.Code)
			assert.Equal(t, clEndpointRefusal, d.Message)
			assert.Equal(t, "endpoints", d.Resource)
			if assert.NotNil(t, d.Limit) {
				assert.Equal(t, 10, *d.Limit)
			}
			assert.Equal(t, "personal", d.RequiredEdition)

			assert.Len(t, e.endpoints(), tc.labelled+tc.accepted, "a refused creation leaves nothing behind")
			assert.Empty(t, e.limited(), "a manual creation is refused, never stored as limited")
		})
	}
}

// FR-002, FR-004, FR-014: the certificate cap counts manual and label monitors alike.
func TestCommunityLimit_ManualCertificateCreationCountsLabelMonitors(t *testing.T) {
	for name, tc := range map[string]struct{ labelled, accepted int }{
		"five label certificates": {5, 0},
		"four label certificates": {4, 1},
		"two label certificates":  {2, 3},
	} {
		t.Run(name, func(t *testing.T) {
			clPinEdition(t, extension.Community)
			e := clOpen(t, "", nil)
			e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, tc.labelled)...))

			for i := range tc.accepted {
				rec := e.createCertificate(1 + i)
				require.Equal(t, http.StatusCreated, rec.Code, "creation %d: %s", i, rec.Body.String())
			}
			d := e.refusal(e.createCertificate(100))
			assert.Equal(t, "QUOTA_EXCEEDED", d.Code)
			assert.Equal(t, clCertificateRefusal, d.Message)
			assert.Equal(t, "certificates", d.Resource)
			if assert.NotNil(t, d.Limit) {
				assert.Equal(t, 5, *d.Limit)
			}
			assert.Equal(t, "personal", d.RequiredEdition)
		})
	}
}

// US1 scenarios 1, 2, 3, 4 and 5, FR-003, FR-004, FR-017, SC-001, edge cases
// "au-dessus du plafond par antériorité" and "droits fixés une seule fois".
func TestCommunityLimit_UpgradeKeepsWhatWasMonitored(t *testing.T) {
	clPinEdition(t, extension.Pro)
	old := clOpen(t, "", nil)
	old.declareAll(clNames("svc", 1, 12)...)
	old.declare("svc-13-batch", 2)
	old.a.endpointSvc.HandleContainerStop(old.ctx, clExt("svc-13-batch", 0))
	old.a.certSvc.SyncFromLabels(old.ctx, "ext-certs", clCertLabels(clCertHosts(1, 7)...))
	history := old.find("svc-03").ID
	old.a.endpointSvc.ProcessCheckResult(old.ctx, history, endpoint.CheckResult{
		EndpointID: history, Success: true, ResponseTimeMs: 12, Timestamp: time.Now(),
	})
	historyBefore := old.checksTotal(history)
	require.Positive(t, historyBefore)
	path := old.dbPath
	old.stop()
	clRollBackTo43(t, path)

	clPinEdition(t, extension.Community)
	e := clOpen(t, path, nil)
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	require.NoError(t, e.a.certSvc.ApplyLimit(e.ctx))

	require.Len(t, e.endpoints(), 14)
	assert.Empty(t, e.limited(), "the 14 endpoints stay monitored")
	certMonitored, certLimited := e.certSplit()
	assert.Len(t, certMonitored, 7)
	assert.Empty(t, certLimited, "the 7 label certificates stay monitored")
	assert.GreaterOrEqual(t, e.checksTotal(history), historyBefore, "history is kept")

	e.a.endpointSvc.HandleContainerStart(e.ctx, "svc-13-batch", clExt("svc-13-batch", 0), clTCPLabels(e.target, 2))
	assert.False(t, e.find("svc-13-batch").Limited, "a container stopped at the update keeps its endpoints")
	assert.False(t, e.find("svc-13-batch/maintenant.endpoint.1.tcp").Limited)

	e.declare("svc-20-new", 1)
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "a new endpoint past the cap is not monitored")
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-new-certs", clCertLabels("new.example.com"))
	_, certLimited = e.certSplit()
	assert.Equal(t, []string{"new.example.com"}, certLimited)

	svc05 := e.find("svc-05").ID
	e.a.endpointSvc.HandleContainerDestroy(e.ctx, clExt("svc-05", 0))
	e.a.endpointSvc.HandleContainerStart(e.ctx, "svc-05", clExt("svc-05", 1), clTCPLabels(e.target, 1))
	assert.Equal(t, svc05, e.find("svc-05").ID, "recreated under the same name, it is the same endpoint")
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "a recreated grandfathered endpoint keeps its slot")

	d := e.refusal(e.createEndpoint("manual"))
	assert.Equal(t, clEndpointRefusal, d.Message, "past the cap by grandfathering, manual creation is refused")

	e.assertQuota("endpoints", 14, 10, 1)
	e.assertQuota("certificates", 7, 5, 1)

	e.stop()
	again := clOpen(t, path, nil)
	require.NoError(t, again.a.endpointSvc.ApplyLimit(again.ctx))
	require.NoError(t, again.a.certSvc.ApplyLimit(again.ctx))
	assert.Equal(t, []string{"svc-20-new"}, again.limited(), "rights are fixed once, at the first start only")
	_, certLimited = again.certSplit()
	assert.Equal(t, []string{"new.example.com"}, certLimited)
}

// US1 scenario 6, FR-004: removing labels from a grandfathered instance keeps the
// rest monitored, and nothing new gets in while the total stays at 10 or more.
func TestCommunityLimit_RemovingGrandfatheredLabelsKeepsTheRest(t *testing.T) {
	e := clUpgraded(t, 14, 0)
	require.Empty(t, e.limited())

	var removed []string
	for _, n := range []string{"svc-02", "svc-07", "svc-12"} {
		removed = append(removed, e.find(n).ID)
		e.a.endpointSvc.SyncEndpoints(e.ctx, n, clExt(n, 0), map[string]string{})
	}
	monitored, limited := e.split()
	assert.Len(t, monitored, 11, "the other 11 stay monitored")
	assert.Empty(t, limited)

	e.declare("svc-20-new", 1)
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "11 still exceed the cap: nothing new is monitored")

	for _, id := range removed {
		e.exec(`UPDATE endpoints SET last_seen_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).Unix(), id)
	}
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	assert.Equal(t, []string{"svc-20-new"}, e.limited(), "the freed rights are not handed to a newcomer while the total stays above 10")
	assert.Equal(t, 11, e.a.checkEngine.ActiveCount())
}

// US1 scenario 7, FR-010: the right disappears with the purge, and what comes back
// afterwards is new.
func TestCommunityLimit_RightDisappearsWithThePurge(t *testing.T) {
	e := clUpgraded(t, 14, 7)
	require.Empty(t, e.limited())

	gone := e.find("svc-05").ID
	e.a.endpointSvc.HandleContainerDestroy(e.ctx, clExt("svc-05", 0))
	e.exec(`UPDATE endpoints SET last_seen_at=? WHERE id=?`, time.Now().Add(-40*24*time.Hour).Unix(), gone)
	e.a.certSvc.HandleContainerDestroy(e.ctx, "ext-certs")
	e.exec(`UPDATE cert_monitors SET inactive_since=? WHERE inactive_since IS NOT NULL`, time.Now().Add(-40*24*time.Hour).Unix())
	clRunRetention(t, e, func() bool {
		ep, err := e.a.epStore.GetEndpointByID(e.ctx, gone)
		if err != nil || ep != nil {
			return false
		}
		all, err := e.a.certStore.ListMonitors(e.ctx, certificate.ListCertificatesOpts{IncludeInactive: true})
		return err == nil && len(all) == 0
	})

	e.a.endpointSvc.HandleContainerStart(e.ctx, "svc-05", clExt("svc-05", 1), clTCPLabels(e.target, 1))
	assert.Equal(t, []string{"svc-05"}, e.limited(), "back after 40 days, it is a new endpoint past the cap")

	e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs-2", clCertLabels(clCertHosts(1, 7)...))
	monitored, limited := e.certSplit()
	assert.Len(t, monitored, 5, "back after 40 days, the certificates are new and get the free slots only")
	assert.Len(t, limited, 2)
}

// US1 scenario 2, SC-005, FR-005, edge case "docker compose up": recreating a
// container under the same name, twenty times in a row, never changes what is
// monitored nor loses history, and a waiting endpoint never takes the slot.
func TestCommunityLimit_RecreatingAContainerNeverChangesWhatIsMonitored(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 11)...)
	require.Equal(t, []string{"svc-11"}, e.limited())

	id := e.find("svc-03").ID
	e.a.endpointSvc.ProcessCheckResult(e.ctx, id, endpoint.CheckResult{EndpointID: id, Success: true, ResponseTimeMs: 7, Timestamp: time.Now()})
	history := e.checksTotal(id)
	require.Positive(t, history)

	for gen := 1; gen <= 20; gen++ {
		e.a.endpointSvc.HandleContainerDestroy(e.ctx, clExt("svc-03", gen-1))
		e.a.endpointSvc.HandleContainerStart(e.ctx, "svc-03", clExt("svc-03", gen), clTCPLabels(e.target, 1))
		monitored, limited := e.split()
		require.Equal(t, clNames("svc", 1, 10), monitored, "recreation %d", gen)
		require.Equal(t, []string{"svc-11"}, limited, "recreation %d", gen)
	}
	assert.Equal(t, id, e.find("svc-03").ID)
	assert.GreaterOrEqual(t, e.checksTotal(id), history, "history survives the recreations")
}

// FR-005, edge case "un conteneur est renommé": a destroyed endpoint keeps its
// slot ten minutes while others wait, a renamed container is a new identity, and
// once the delay is over the oldest waiting endpoint takes the slot.
func TestCommunityLimit_VanishedEndpointHoldsItsSlotTenMinutesWhileOthersWait(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 11)...)
	require.Equal(t, []string{"svc-11"}, e.limited())
	vanished := e.find("svc-05").ID

	e.a.endpointSvc.HandleContainerDestroy(e.ctx, clExt("svc-05", 0))
	assert.Equal(t, []string{"svc-11"}, e.limited(), "a waiting endpoint does not take the slot of a container being recreated")

	e.a.endpointSvc.HandleContainerStart(e.ctx, "zz-svc-05-renamed", clExt("zz-svc-05-renamed", 0), clTCPLabels(e.target, 1))
	assert.Equal(t, []string{"svc-11", "zz-svc-05-renamed"}, e.limited(), "a renamed container is a new endpoint")

	e.exec(`UPDATE endpoints SET last_seen_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).Unix(), vanished)
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	monitored, limited := e.split()
	assert.Equal(t, []string{"zz-svc-05-renamed"}, limited, "after ten minutes the slot goes to the oldest waiting")
	assert.Contains(t, monitored, "svc-11")
	assert.Equal(t, 10, e.a.checkEngine.ActiveCount())
}

// FR-005: without anyone waiting, a destroyed endpoint frees its slot at once.
func TestCommunityLimit_VanishedEndpointFreesItsSlotWhenNobodyWaits(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 10)...)
	require.Empty(t, e.limited())

	vanished := e.find("svc-05").ID

	e.a.endpointSvc.HandleContainerDestroy(e.ctx, clExt("svc-05", 0))
	e.declare("svc-11", 1)

	assert.Empty(t, e.limited(), "the freed slot is taken by the next endpoint")
	assert.Equal(t, 10, e.a.checkEngine.ActiveCount())

	e.exec(`UPDATE endpoints SET last_seen_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).Unix(), vanished)
	e.declare("svc-12", 1)
	assert.Equal(t, []string{"svc-12"}, e.limited(), "the cap still holds once the slot is taken")
}

// FR-005: stopping, restarting or pausing a container keeps its slot.
func TestCommunityLimit_StoppedContainerKeepsItsSlot(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 11)...)
	require.Equal(t, []string{"svc-11"}, e.limited())

	e.a.endpointSvc.HandleContainerStop(e.ctx, clExt("svc-04", 0))
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	assert.Equal(t, []string{"svc-11"}, e.limited(), "a stopped container keeps its slot")
	assert.False(t, e.find("svc-04").Limited)

	e.a.endpointSvc.HandleContainerStart(e.ctx, "svc-04", clExt("svc-04", 0), clTCPLabels(e.target, 1))
	assert.Equal(t, []string{"svc-11"}, e.limited())
	assert.Equal(t, 10, e.a.checkEngine.ActiveCount())
}

// FR-009, US2 scenario 10: a freed slot goes to the oldest waiting element,
// without any action from the user.
func TestCommunityLimit_FreedSlotGoesToTheOldestWaiting(t *testing.T) {
	t.Run("manual endpoint deleted", func(t *testing.T) {
		clPinEdition(t, extension.Community)
		e := clOpen(t, "", nil)
		for i := range 3 {
			require.Equal(t, http.StatusCreated, e.createEndpoint(fmt.Sprintf("manual-%d", i)).Code)
		}
		e.declareAll(clNames("svc", 1, 10)...)
		require.Equal(t, clNames("svc", 8, 10), e.limited())

		rec := e.call(http.MethodDelete, "/api/v1/endpoints/"+e.find("manual:manual-1").ID, "")
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

		assert.Equal(t, clNames("svc", 9, 10), e.limited(), "the oldest waiting endpoint takes the slot")
		assert.Equal(t, 10, e.a.checkEngine.ActiveCount())
	})

	t.Run("label removed", func(t *testing.T) {
		clPinEdition(t, extension.Community)
		e := clOpen(t, "", nil)
		e.declareAll(clNames("svc", 1, 13)...)
		require.Equal(t, clNames("svc", 11, 13), e.limited())
		removed := e.find("svc-02").ID

		e.a.endpointSvc.SyncEndpoints(e.ctx, "svc-02", clExt("svc-02", 0), map[string]string{})
		e.exec(`UPDATE endpoints SET last_seen_at=? WHERE id=?`, time.Now().Add(-11*time.Minute).Unix(), removed)
		require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))

		monitored, limited := e.split()
		assert.NotContains(t, monitored, "svc-02")
		assert.Contains(t, monitored, "svc-11")
		assert.Equal(t, clNames("svc", 12, 13), limited)
	})

	t.Run("manual certificate deleted", func(t *testing.T) {
		clPinEdition(t, extension.Community)
		e := clOpen(t, "", nil)
		var manual []string
		for i := range 2 {
			var body struct {
				Certificate clCert `json:"certificate"`
			}
			e.decode(e.createCertificate(1+i), http.StatusCreated, &body)
			manual = append(manual, body.Certificate.ID)
		}
		e.a.certSvc.SyncFromLabels(e.ctx, "ext-a", clCertLabels("a1.example.com", "a2.example.com", "a3.example.com"))
		e.a.certSvc.SyncFromLabels(e.ctx, "ext-b", clCertLabels("b1.example.com", "b2.example.com"))
		_, limited := e.certSplit()
		require.Equal(t, []string{"b1.example.com", "b2.example.com"}, limited)

		rec := e.call(http.MethodDelete, "/api/v1/certificates/"+manual[0], "")
		require.Less(t, rec.Code, 300, rec.Body.String())

		_, limited = e.certSplit()
		assert.Equal(t, []string{"b2.example.com"}, limited, "the oldest waiting certificate takes the slot")
	})
}

// US2 scenario 6, FR-011, FR-020, SC-004: moving to Personal or Pro monitors
// everything left out, without a restart, and announces each change.
func TestCommunityLimit_PaidEditionMonitorsEverythingWithoutRestart(t *testing.T) {
	for _, paid := range []extension.Edition{extension.Personal, extension.Pro} {
		t.Run(string(paid), func(t *testing.T) {
			clPinEdition(t, extension.Community)
			e := clOpen(t, "", nil)
			e.declareAll(clNames("svc", 1, 13)...)
			e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, 7)...))
			require.Equal(t, clNames("svc", 11, 13), e.limited())
			_, certLimited := e.certSplit()
			require.Len(t, certLimited, 2)
			waitingEndpoints := map[string]bool{}
			for _, k := range clNames("svc", 11, 13) {
				waitingEndpoints[e.find(k).ID] = false
			}
			waitingCerts := map[string]bool{}
			for _, h := range certLimited {
				waitingCerts[e.cert(h).ID] = false
			}
			e.drain()

			clPinEdition(t, paid)
			require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
			require.NoError(t, e.a.certSvc.ApplyLimit(e.ctx))

			assert.Empty(t, e.limited())
			_, certLimited = e.certSplit()
			assert.Empty(t, certLimited)
			assert.Equal(t, 13, e.a.checkEngine.ActiveCount(), "the endpoints left out are checked now")

			events := e.drain()
			assert.Equal(t, waitingEndpoints, clLimitFlags(t, events, event.EndpointLimitChanged, "endpoint_id"))
			assert.Equal(t, waitingCerts, clLimitFlags(t, events, event.CertificateLimitChanged, "monitor_id"))

			e.assertQuota("endpoints", 13, extension.Unlimited, 0)

			e.declareAll(clNames("more", 1, 25)...)
			assert.Empty(t, e.limited(), "a paid edition has no cap, whatever the origin")
		})
	}
}

// US2 scenario 9, FR-012, edge cases "hôte distant", "alerte ouverte" and
// "abonnement qui reprend": back to Community, the first ten discovered stay
// monitored, remote ones counted, the others are left out with their alerts
// resolved and their history kept; back to Pro, everything is monitored again.
func TestCommunityLimit_DowngradeLimitsBeyondTheCapAndKeepsHistory(t *testing.T) {
	clPinEdition(t, extension.Pro)
	e := clOpen(t, "", nil)
	require.NoError(t, store.NewAgentStore(e.a.db).Insert(e.ctx, &agent.Agent{
		AgentID: clAgentID, Hostname: "nas", Label: "nas", OSArch: "linux/amd64",
		AgentVersion: "1.0.0", DetectedRuntime: "docker", Status: "active", CreatedAt: time.Now().UTC(),
	}))

	local := clNames("svc", 1, 28)
	victimName := local[14]
	for _, n := range local {
		if n == victimName {
			e.a.endpointSvc.SyncEndpoints(e.ctx, n, clExt(n, 0), clTCPLabels("127.0.0.1:1", 1))
			continue
		}
		e.declare(n, 1)
	}
	e.a.endpointSvc.SyncAgentEndpoints(e.ctx, clAgentID, "nas-web", "ext-nas-web", clTCPLabels("10.20.0.1:80", 1))
	e.a.endpointSvc.SyncAgentEndpoints(e.ctx, clAgentID, "nas-db", "ext-nas-db", clTCPLabels("10.20.0.2:5432", 1))
	require.Len(t, e.endpoints(), 30)
	require.Empty(t, e.limited())

	order := append([]string(nil), local...)
	rand.New(rand.NewSource(51)).Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	order = append(append([]string{"nas-web"}, order...), "nas-db")
	for i, k := range order {
		if k == victimName && i < 10 {
			order[i], order[15] = order[15], order[i]
		}
	}
	base := time.Now().Add(-48 * time.Hour)
	ids := map[string]string{}
	for i, k := range order {
		ids[k] = e.find(k).ID
		e.exec(`UPDATE endpoints SET first_seen_at=? WHERE id=?`, base.Add(time.Duration(i)*time.Minute).Unix(), ids[k])
	}
	wantMonitored := append([]string(nil), order[:10]...)
	sort.Strings(wantMonitored)
	wantLimited := append([]string(nil), order[10:]...)
	sort.Strings(wantLimited)

	victim := ids[victimName]
	for range 3 {
		e.a.endpointSvc.ProcessCheckResult(e.ctx, victim, endpoint.CheckResult{
			EndpointID: victim, ErrorMessage: "connection refused", Timestamp: time.Now(),
		})
	}
	alerts := store.NewAlertStore(e.a.db)
	victimAlerted := func() bool {
		active, err := alerts.ListActiveAlerts(e.ctx)
		if err != nil {
			return false
		}
		for _, al := range active {
			if al.EntityType == "endpoint" && al.EntityID == victim {
				return true
			}
		}
		return false
	}
	require.Eventually(t, victimAlerted, 10*time.Second, 50*time.Millisecond, "the victim is in alert before the downgrade")
	_, err := e.a.certSvc.EnsureAutoDetected(e.ctx, victim, "https://victim.internal:8443")
	require.NoError(t, err)
	component, err := e.a.statusCompStore.CreateComponent(e.ctx, &status.Component{
		CompositionMode: status.CompositionExplicit, DisplayName: "Victim", Visible: true,
		Monitors: []status.MonitorRef{{Type: "endpoint", ID: victim}},
	})
	require.NoError(t, err)
	history := e.checksTotal(victim)
	require.Positive(t, history)
	e.drain()

	clPinEdition(t, extension.Community)
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))

	monitored, limited := e.split()
	assert.Equal(t, wantMonitored, monitored, "the first ten discovered stay monitored, the remote one counted")
	assert.Equal(t, wantLimited, limited)
	for _, ep := range e.endpoints() {
		if ep.Limited {
			assert.Equal(t, "unknown", ep.Status, clKey(ep))
		}
	}
	assert.Equal(t, 9, e.a.checkEngine.ActiveCount(), "nine local endpoints are checked; the remote one is the agent's")

	changed := clLimitFlags(t, e.drain(), event.EndpointLimitChanged, "endpoint_id")
	wantChanged := map[string]bool{}
	for _, k := range wantLimited {
		wantChanged[ids[k]] = true
	}
	assert.Equal(t, wantChanged, changed)

	assert.Eventually(t, func() bool { return !victimAlerted() }, 5*time.Second, 50*time.Millisecond,
		"the alert of an endpoint left out is resolved")
	m, err := e.a.certStore.GetMonitorByEndpointID(e.ctx, victim)
	require.NoError(t, err)
	assert.Nil(t, m, "the certificate detected on it goes with it")
	comp, err := e.a.statusCompStore.GetComponent(e.ctx, component)
	require.NoError(t, err)
	for _, ref := range comp.Monitors {
		assert.NotEqual(t, victim, ref.ID, "an endpoint left out leaves the status page")
	}
	assert.GreaterOrEqual(t, e.checksTotal(victim), history, "its history is kept")

	nasDB := ids["nas-db"]
	require.NoError(t, e.a.endpointSvc.HandleAgentEvent(e.ctx, clAgentID, &agentpb.EndpointEvent{
		Url: "10.20.0.2:5432", Status: agentpb.EndpointStatus_ENDPOINT_STATUS_DOWN, ErrorMessage: "refused",
	}, agentevent.Meta{ObservedAt: time.Now()}))
	assert.Zero(t, e.checksTotal(nasDB), "a result pushed for an endpoint left out is ignored")
	assert.Equal(t, "unknown", e.find("nas-db").Status)

	clPinEdition(t, extension.Pro)
	require.NoError(t, e.a.endpointSvc.ApplyLimit(e.ctx))
	assert.Empty(t, e.limited(), "back on Pro, everything is monitored again")
	assert.GreaterOrEqual(t, e.checksTotal(victim), history)
}

// US2 scenario 9, FR-012 for certificates: back to Community, the first five
// declared certificates stay monitored, the others are left out and never scanned.
func TestCommunityLimit_DowngradeLimitsCertificatesBeyondTheCap(t *testing.T) {
	clPinEdition(t, extension.Pro)
	e := clOpen(t, "", nil)
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, 8)...))
	monitored, limited := e.certSplit()
	require.Len(t, monitored, 8)
	require.Empty(t, limited)
	base := time.Now().Add(-48 * time.Hour)
	order := []string{"cert-07.example.com", "cert-02.example.com", "cert-05.example.com", "cert-01.example.com",
		"cert-08.example.com", "cert-03.example.com", "cert-06.example.com", "cert-04.example.com"}
	for i, h := range order {
		e.exec(`UPDATE cert_monitors SET created_at=? WHERE id=?`, base.Add(time.Duration(i)*time.Minute).Unix(), e.cert(h).ID)
	}
	e.drain()

	clPinEdition(t, extension.Community)
	require.NoError(t, e.a.certSvc.ApplyLimit(e.ctx))

	wantMonitored := append([]string(nil), order[:5]...)
	sort.Strings(wantMonitored)
	wantLimited := append([]string(nil), order[5:]...)
	sort.Strings(wantLimited)
	monitored, limited = e.certSplit()
	assert.Equal(t, wantMonitored, monitored, "the first five declared stay monitored")
	assert.Equal(t, wantLimited, limited)

	wantChanged := map[string]bool{}
	for _, h := range wantLimited {
		wantChanged[e.cert(h).ID] = true
	}
	assert.Equal(t, wantChanged, clLimitFlags(t, e.drain(), event.CertificateLimitChanged, "monitor_id"))

	due, err := e.a.certStore.ListDueScheduledMonitors(e.ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	var scheduled []string
	for _, m := range due {
		scheduled = append(scheduled, m.Hostname)
	}
	sort.Strings(scheduled)
	assert.Equal(t, wantMonitored, scheduled)
}

// US3 scenarios 1 and 3, FR-002, FR-013, SC-002: under the cap nothing changes,
// the certificates of HTTPS endpoints are detected and do not count.
func TestCommunityLimit_UnderTheCapNothingChanges(t *testing.T) {
	servers := []clTLS{clTLSServer(t), clTLSServer(t), clTLSServer(t), clTLSServer(t)}
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)

	e.declareAll("svc-01", "svc-02")
	var https []string
	for i, s := range servers {
		name := fmt.Sprintf("web-%d", i+1)
		e.a.endpointSvc.SyncEndpoints(e.ctx, name, clExt(name, 0), clHTTPSLabels(s.url))
		https = append(https, e.find(name).ID)
	}
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, 5)...))

	require.Eventually(t, func() bool {
		for _, id := range https {
			if m, err := e.a.certStore.GetMonitorByEndpointID(e.ctx, id); err != nil || m == nil {
				return false
			}
		}
		return true
	}, 15*time.Second, 50*time.Millisecond, "the four HTTPS certificates are detected")

	assert.Len(t, e.endpoints(), 6)
	assert.Empty(t, e.limited())
	assert.Equal(t, 6, e.a.checkEngine.ActiveCount())
	for _, c := range e.certificates() {
		assert.False(t, c.Limited, "%s:%d (%s)", c.Hostname, c.Port, c.Source)
	}
	e.assertQuota("endpoints", 6, 10, 0)
	e.assertQuota("certificates", 5, 5, 0)

	events := e.drain()
	for id, limited := range clLimitFlags(t, events, event.EndpointDiscovered, "endpoint_id") {
		assert.False(t, limited, id)
	}
	for id, limited := range clLimitFlags(t, events, event.CertificateCreated, "monitor_id") {
		assert.False(t, limited, id)
	}
	for _, ev := range events {
		assert.NotEqual(t, event.EndpointLimitChanged, ev.Type)
		assert.NotEqual(t, event.CertificateLimitChanged, ev.Type)
	}
}

// US2 scenario 5, FR-006, FR-007: a sixth label certificate is listed as not
// monitored, never scanned, and every surface says so.
func TestCommunityLimit_SixthLabelCertificateIsNotMonitored(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-a", clCertLabels(clCertHosts(1, 5)...))
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-b", clCertLabels("cert-06.example.com"))

	monitored, limited := e.certSplit()
	assert.Equal(t, clCertHosts(1, 5), monitored)
	assert.Equal(t, []string{"cert-06.example.com"}, limited)
	sixth := e.cert("cert-06.example.com")

	var detail struct {
		Certificate clCert `json:"certificate"`
	}
	e.decode(e.call(http.MethodGet, "/api/v1/certificates/"+sixth.ID, ""), http.StatusOK, &detail)
	assert.True(t, detail.Certificate.Limited)

	created := clLimitFlags(t, e.drain(), event.CertificateCreated, "monitor_id")
	assert.Equal(t, true, created[sixth.ID], "the creation event carries the state")
	assert.Equal(t, false, created[e.cert("cert-01.example.com").ID])

	d := e.refusal(e.call(http.MethodPost, "/api/v1/certificates/"+sixth.ID+"/check", ""))
	assert.Equal(t, "QUOTA_EXCEEDED", d.Code)
	assert.Equal(t, "certificates", d.Resource)

	due, err := e.a.certStore.ListDueScheduledMonitors(e.ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	var scheduled []string
	for _, m := range due {
		scheduled = append(scheduled, m.Hostname)
	}
	sort.Strings(scheduled)
	assert.Equal(t, clCertHosts(1, 5), scheduled, "an unmonitored certificate is never scanned")

	e.assertQuota("certificates", 5, 5, 1)
}

// FR-005 for certificates: a label certificate keeps its slot and its history
// when its container is recreated, and a waiting one does not take it.
func TestCommunityLimit_LabelCertificateKeepsItsSlotAcrossRecreation(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	hosts := clCertHosts(1, 5)
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-a", clCertLabels(hosts...))
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-b", clCertLabels("cert-06.example.com"))
	before := map[string]string{}
	for _, h := range hosts {
		before[h] = e.cert(h).ID
	}
	third := before["cert-03.example.com"]
	_, err := e.a.certStore.InsertCheckResult(e.ctx, &certificate.CertCheckResult{
		MonitorID: third, CheckedAt: time.Now(), ErrorMessage: "connection refused",
	})
	require.NoError(t, err)

	e.a.certSvc.HandleContainerDestroy(e.ctx, "ext-a")
	monitored, limited := e.certSplit()
	assert.Empty(t, monitored, "the monitors of a destroyed container leave the list")
	assert.Equal(t, []string{"cert-06.example.com"}, limited, "a waiting certificate does not take their slots")

	e.a.certSvc.SyncFromLabels(e.ctx, "ext-a-2", clCertLabels(hosts...))
	monitored, limited = e.certSplit()
	assert.Equal(t, hosts, monitored)
	assert.Equal(t, []string{"cert-06.example.com"}, limited)
	for _, h := range hosts {
		assert.Equal(t, before[h], e.cert(h).ID, "%s is the same monitor", h)
	}
	_, total, err := e.a.certSvc.ListCheckResults(e.ctx, third, certificate.ListChecksOpts{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total, "its history survives the recreation")
}

// FR-006: an element left out is not offered on the status pages.
func TestCommunityLimit_UnmonitoredElementsStayOffStatusPages(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", nil)
	e.declareAll(clNames("svc", 1, 11)...)
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, 6)...))
	limitedEndpoint := e.find("svc-11").ID
	limitedCert := e.cert("cert-06.example.com").ID
	require.True(t, e.find("svc-11").Limited)

	for _, kind := range []string{"endpoint", "certificate"} {
		_, err := e.a.statusCompStore.CreateComponent(e.ctx, &status.Component{
			CompositionMode: status.CompositionMatchAll, MatchAllType: kind, DisplayName: "All " + kind, Visible: true,
		})
		require.NoError(t, err)
	}

	page, err := e.a.statusSvc.GetPageData(e.ctx)
	require.NoError(t, err)
	counts := map[string]int{}
	for _, c := range page.Components {
		for _, m := range c.Monitors {
			counts[m.Type]++
			assert.NotEqual(t, limitedEndpoint, m.ID, "an unmonitored endpoint is not on the status page")
			assert.NotEqual(t, limitedCert, m.ID, "an unmonitored certificate is not on the status page")
		}
	}
	assert.Equal(t, map[string]int{"endpoint": 10, "certificate": 5}, counts)
}

// Edge cases "deux labels qui déclarent la même cible" and "un endpoint non
// surveillé dont le label est retiré disparaît de la liste".
func TestCommunityLimit_IdentityEdgeCases(t *testing.T) {
	t.Run("two labels on one target count twice", func(t *testing.T) {
		clPinEdition(t, extension.Community)
		e := clOpen(t, "", nil)
		e.declareAll(clNames("svc", 1, 9)...)
		e.declare("zz-dual", 2)

		assert.Len(t, e.endpoints(), 11)
		_, limited := e.split()
		require.Len(t, limited, 1)
		assert.True(t, strings.HasPrefix(limited[0], "zz-dual"), limited[0])
	})

	t.Run("an unmonitored endpoint whose label is removed disappears", func(t *testing.T) {
		clPinEdition(t, extension.Community)
		e := clOpen(t, "", nil)
		e.declareAll(clNames("svc", 1, 11)...)
		require.Equal(t, []string{"svc-11"}, e.limited())

		e.a.endpointSvc.SyncEndpoints(e.ctx, "svc-11", clExt("svc-11", 0), map[string]string{})

		monitored, limited := e.split()
		assert.Empty(t, limited)
		assert.Equal(t, clNames("svc", 1, 10), monitored)
	})
}

// FR-007, FR-017 on the MCP surface: list_endpoints and list_certificates carry
// the state, get_edition reports the unmonitored count.
func TestCommunityLimit_MCPReportsTheState(t *testing.T) {
	clPinEdition(t, extension.Community)
	e := clOpen(t, "", func(c *Config) {
		c.MCP.Enabled = true
		c.MCP.AllowUnauthenticated = true
	})
	e.declareAll(clNames("svc", 1, 12)...)
	e.a.certSvc.SyncFromLabels(e.ctx, "ext-certs", clCertLabels(clCertHosts(1, 6)...))

	cs := mcpSession(t, e.a, "")
	call := func(name string, into any) {
		t.Helper()
		res, err := cs.CallTool(e.ctx, &gomcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		require.NoError(t, err)
		require.False(t, res.IsError, text(t, res))
		require.NoError(t, json.Unmarshal([]byte(text(t, res)), into), text(t, res))
	}

	var endpoints []map[string]any
	call("list_endpoints", &endpoints)
	limitedByContainer := map[string]any{}
	for _, ep := range endpoints {
		limitedByContainer[fmt.Sprint(ep["container_name"])] = ep["limited"]
	}
	assert.Equal(t, true, limitedByContainer["svc-11"])
	assert.Equal(t, true, limitedByContainer["svc-12"])
	assert.Equal(t, false, limitedByContainer["svc-01"])

	var certs []map[string]any
	call("list_certificates", &certs)
	limitedByHost := map[string]any{}
	for _, c := range certs {
		limitedByHost[fmt.Sprint(c["hostname"])] = c["limited"]
	}
	assert.Equal(t, true, limitedByHost["cert-06.example.com"])
	assert.Equal(t, false, limitedByHost["cert-01.example.com"])

	var edition struct {
		Quotas map[string]clQuota `json:"quotas"`
	}
	call("get_edition", &edition)
	for resource, want := range map[string][3]int{"endpoints": {10, 10, 2}, "certificates": {5, 5, 1}} {
		q, ok := edition.Quotas[resource]
		require.True(t, ok, resource)
		assert.Equal(t, want[0], q.Used, resource)
		assert.Equal(t, want[1], q.Limit, resource)
		if assert.NotNil(t, q.Unmonitored, resource) {
			assert.Equal(t, want[2], *q.Unmonitored, resource)
		}
	}

	tools, err := cs.ListTools(e.ctx, nil)
	require.NoError(t, err)
	for _, tool := range tools.Tools {
		if tool.Name == "list_endpoints" || tool.Name == "list_certificates" {
			assert.Contains(t, tool.Description, "limited", "%s documents the state", tool.Name)
		}
	}
}
