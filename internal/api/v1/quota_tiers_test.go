// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/heartbeat"
	"github.com/kolapsis/maintenant/internal/status"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

// newCreator returns a function that performs the i-th creation of r through its handler.
func newCreator(t *testing.T, r extension.Resource) func(i int) *httptest.ResponseRecorder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)

	post := func(h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec
	}

	switch r {
	case extension.ResourceEndpoints:
		svc := endpoint.NewService(endpoint.Deps{
			Store:  store.NewEndpointStore(db),
			Engine: endpoint.NewCheckEngine(nil, logger),
			Logger: logger,
		})
		h := NewEndpointHandler(svc, nil)
		return func(i int) *httptest.ResponseRecorder {
			return post(h.HandleCreateEndpoint, "/api/v1/endpoints",
				fmt.Sprintf(`{"name":"ep-%d","target":"127.0.0.1:%d","endpoint_type":"tcp"}`, i, 1000+i))
		}
	case extension.ResourceHeartbeats:
		h := NewHeartbeatHandler(heartbeat.NewService(heartbeat.Deps{Store: store.NewHeartbeatStore(db), Logger: logger}))
		return func(i int) *httptest.ResponseRecorder {
			return post(h.HandleCreate, "/api/v1/heartbeats",
				fmt.Sprintf(`{"name":"hb-%d","interval_seconds":300,"grace_seconds":60}`, i))
		}
	case extension.ResourceCertificates:
		h := NewCertificateHandler(certificate.NewService(certificate.Deps{Store: store.NewCertificateStore(db), Logger: logger}))
		return func(i int) *httptest.ResponseRecorder {
			return post(h.HandleCreate, "/api/v1/certificates",
				fmt.Sprintf(`{"hostname":"127.0.0.1","port":%d}`, 1+i))
		}
	case extension.ResourceStatusComponents:
		components := store.NewStatusComponentStore(db)
		svc := status.NewService(status.Deps{Components: components, Logger: logger})
		h := NewStatusAdminHandler(components, nil, nil, nil, svc, nil, nil)
		return func(i int) *httptest.ResponseRecorder {
			return post(h.HandleCreateComponent, "/api/v1/status/components",
				fmt.Sprintf(`{"display_name":"c-%d","composition_mode":"match-all","match_all_type":"container"}`, i))
		}
	case extension.ResourceAgentHosts:
		agents := store.NewAgentStore(db)
		h := NewAgentHandler(agents, nil, nil, logger, "grpcs://example.test:8443", "127.0.0.1:8443", time.Minute, nil)
		return func(i int) *httptest.ResponseRecorder {
			rec := post(h.HandleCreateEnrollmentToken, "/api/v1/agents/enrollment-tokens", `{"ttl_hours":1}`)
			if rec.Code == http.StatusCreated {
				require.NoError(t, agents.Insert(context.Background(), &agent.Agent{
					AgentID:         fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1),
					Hostname:        fmt.Sprintf("host-%d", i+1),
					Label:           fmt.Sprintf("host-%d", i+1),
					OSArch:          "linux/amd64",
					AgentVersion:    "1.0.0",
					DetectedRuntime: "docker",
					Status:          "active",
					CreatedAt:       time.Now().UTC(),
				}))
			}
			return rec
		}
	}
	t.Fatalf("no creator for %q", r)
	return nil
}

// TestCreate_EveryCappedResourceInEveryEdition fills each resource to the running
// edition's cap through its handler and checks the next creation: refused while
// the cap is finite, accepted past the next finite cap when it is not.
func TestCreate_EveryCappedResourceInEveryEdition(t *testing.T) {
	refusal := map[extension.Resource]struct {
		status int
		code   string
	}{
		extension.ResourceEndpoints:        {http.StatusForbidden, "QUOTA_EXCEEDED"},
		extension.ResourceHeartbeats:       {http.StatusForbidden, "QUOTA_EXCEEDED"},
		extension.ResourceCertificates:     {http.StatusForbidden, "QUOTA_EXCEEDED"},
		extension.ResourceStatusComponents: {http.StatusForbidden, "QUOTA_EXCEEDED"},
		extension.ResourceAgentHosts:       {http.StatusConflict, "HOST_LIMIT_REACHED"},
	}
	lifting := map[extension.Edition]map[extension.Resource]extension.Edition{
		extension.Community: {
			extension.ResourceEndpoints:        extension.Personal,
			extension.ResourceHeartbeats:       extension.Personal,
			extension.ResourceCertificates:     extension.Personal,
			extension.ResourceStatusComponents: extension.Personal,
			extension.ResourceAgentHosts:       extension.Personal,
		},
		extension.Personal: {extension.ResourceAgentHosts: extension.Pro},
		extension.Pro:      {},
	}
	tiers := extension.Tiers()

	for edition, refused := range lifting {
		for resource, want := range refusal {
			t.Run(string(edition)+"/"+string(resource), func(t *testing.T) {
				withEdition(t, edition)
				create := newCreator(t, resource)

				limit := extension.Limit(resource)
				fill := limit
				if limit == extension.Unlimited {
					fill = max(tiers[extension.Community][resource], tiers[extension.Personal][resource])
				}
				for i := range fill {
					rec := create(i)
					require.Equal(t, http.StatusCreated, rec.Code, "creation %d: %s", i, rec.Body.String())
				}

				rec := create(fill)
				required, isRefused := refused[resource]
				if !isRefused {
					assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
					assert.Equal(t, extension.Unlimited, limit)
					return
				}

				require.Equal(t, want.status, rec.Code, rec.Body.String())
				detail := decodeRefusal(t, rec)
				assert.Equal(t, want.code, detail.Code)
				assert.Equal(t, string(resource), detail.Resource)
				require.NotNil(t, detail.Limit)
				assert.Equal(t, limit, *detail.Limit)
				assert.Equal(t, string(required), detail.RequiredEdition)
			})
		}
	}
}
