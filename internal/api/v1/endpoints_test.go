// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/endpoint"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

func newEndpointHandler(t *testing.T) *EndpointHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := endpoint.NewService(endpoint.Deps{
		Store:  store.NewEndpointStore(storetest.Open(t, logger)),
		Engine: endpoint.NewCheckEngine(nil, logger),
		Logger: logger,
	})
	return NewEndpointHandler(svc, nil)
}

func sendEndpoint(h http.HandlerFunc, method, path, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if id != "" {
		req.SetPathValue("id", id)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// A target the checker could never probe is refused up front, on creation and on update alike.
func TestEndpointTargetsAreValidated(t *testing.T) {
	h := newEndpointHandler(t)

	for _, body := range []string{
		`{"name":"db","target":"db","endpoint_type":"tcp"}`,
		`{"name":"db","target":"db:99999","endpoint_type":"tcp"}`,
		`{"name":"web","target":"ftp://example.com","endpoint_type":"http"}`,
		`{"name":"web","target":"/health","endpoint_type":"http"}`,
	} {
		rec := sendEndpoint(h.HandleCreateEndpoint, http.MethodPost, "/api/v1/endpoints", "", body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Contains(t, rec.Body.String(), "INVALID_INPUT", body)
	}

	rec := sendEndpoint(h.HandleCreateEndpoint, http.MethodPost, "/api/v1/endpoints", "",
		`{"name":"db","target":"db:5432","endpoint_type":"tcp"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created struct {
		Endpoint struct {
			ID string `json:"id"`
		} `json:"endpoint"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	for _, body := range []string{
		`{"target":"db"}`,
		`{"endpoint_type":"http"}`,
	} {
		rec := sendEndpoint(h.HandleUpdateEndpoint, http.MethodPut, "/api/v1/endpoints/"+created.Endpoint.ID, created.Endpoint.ID, body)
		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
	}
}
