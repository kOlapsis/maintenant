// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 044-label-command-safety, C7: where a label-declared monitor may connect.
//
// On Linux a dial to 0.0.0.0:port reaches the loopback listener on that port, so the
// unspecified address is a destination the guard must refuse that a test can really serve.

const allowLinkLocalEnv = "MAINTENANT_LABEL_ENDPOINTS_ALLOW_LINK_LOCAL"

func countingHTTP(t *testing.T) (*httptest.Server, string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	require.NoError(t, err)
	return srv, port, &hits
}

func sourced(id string, source EndpointSource, epType EndpointType, target string) *Endpoint {
	cfg := DefaultConfig()
	cfg.Timeout = Duration(2 * time.Second)
	return &Endpoint{ID: id, EndpointType: epType, Target: target, Config: cfg, Source: source}
}

// US2 scenario 3, FR-012, SC-003: a label-declared HTTP monitor never connects to an unspecified address.
func TestCheckHTTP_LabelSourceRefusesAnUnspecifiedAddress(t *testing.T) {
	_, port, hits := countingHTTP(t)

	result := CheckHTTP(context.Background(), sourced("ep-label-any", SourceLabel, TypeHTTP, "http://0.0.0.0:"+port+"/"), nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, "probe guard: blocked connection to 0.0.0.0")
	assert.Equal(t, int32(0), hits.Load(), "no request may reach the server")
}

// US2 scenario 3, FR-012: link-local and multicast destinations are refused with the reason, whatever the port or path.
func TestCheckHTTP_LabelSourceRefusesLinkLocalAndMulticast(t *testing.T) {
	for target, ip := range map[string]string{
		"http://169.254.169.254/latest/meta-data/": "169.254.169.254",
		"http://169.254.10.20:8080/":               "169.254.10.20",
		"http://224.0.0.1/":                        "224.0.0.1",
	} {
		t.Run(target, func(t *testing.T) {
			ep := sourced("ep-label-"+ip, SourceLabel, TypeHTTP, target)
			ep.Config.Timeout = Duration(time.Second)

			result := CheckHTTP(context.Background(), ep, nil)

			assert.False(t, result.Success)
			assert.Contains(t, result.ErrorMessage, "probe guard: blocked connection to "+ip)
		})
	}
}

// US2 scenario 4, FR-012: loopback stays reachable for a label-declared monitor.
func TestCheckHTTP_LabelSourceStillReachesLoopback(t *testing.T) {
	srv, _, hits := countingHTTP(t)

	result := CheckHTTP(context.Background(), sourced("ep-label-lo", SourceLabel, TypeHTTP, srv.URL+"/health"), nil)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, int32(1), hits.Load())
}

// US2 scenario 5, FR-014, SC-003: the same destination is attempted for an API-created monitor.
func TestCheckHTTP_StandaloneSourceIsNotGuarded(t *testing.T) {
	_, port, hits := countingHTTP(t)

	result := CheckHTTP(context.Background(), sourced("ep-api-any", SourceStandalone, TypeHTTP, "http://0.0.0.0:"+port+"/"), nil)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, int32(1), hits.Load())
}

// US2 scenario 7, FR-012: a redirect of a label-declared monitor towards a refused address is refused too.
func TestCheckHTTP_LabelSourceRefusesARedirectToABlockedAddress(t *testing.T) {
	_, hiddenPort, hiddenHits := countingHTTP(t)
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://0.0.0.0:"+hiddenPort+"/next", http.StatusFound)
	}))
	t.Cleanup(front.Close)

	label := CheckHTTP(context.Background(), sourced("ep-label-redir", SourceLabel, TypeHTTP, front.URL+"/"), nil)
	assert.False(t, label.Success)
	assert.Contains(t, label.ErrorMessage, "probe guard: blocked connection to 0.0.0.0")
	assert.Equal(t, int32(0), hiddenHits.Load(), "the redirect target must not be reached")

	api := CheckHTTP(context.Background(), sourced("ep-api-redir", SourceStandalone, TypeHTTP, front.URL+"/"), nil)
	require.True(t, api.Success, api.ErrorMessage)
	require.NotNil(t, api.HTTPStatus)
	assert.Equal(t, http.StatusOK, *api.HTTPStatus)
	assert.Equal(t, int32(1), hiddenHits.Load(), "an API monitor follows the redirect as today")
}

// Edge case "moniteur TCP issu de label": the destination rule is the same for TCP.
func TestCheckTCP_LabelSourceRefusesAnUnspecifiedAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	label := CheckTCP(context.Background(), sourced("tcp-label-any", SourceLabel, TypeTCP, "0.0.0.0:"+port), nil)
	assert.False(t, label.Success)
	assert.Contains(t, label.ErrorMessage, "probe guard: blocked connection to 0.0.0.0")
	assert.Equal(t, int32(0), accepted.Load(), "no connection may be opened")

	loopback := CheckTCP(context.Background(), sourced("tcp-label-lo", SourceLabel, TypeTCP, "127.0.0.1:"+port), nil)
	assert.True(t, loopback.Success, loopback.ErrorMessage)

	api := CheckTCP(context.Background(), sourced("tcp-api-any", SourceStandalone, TypeTCP, "0.0.0.0:"+port), nil)
	assert.True(t, api.Success, api.ErrorMessage)
	assert.Eventually(t, func() bool { return accepted.Load() == 2 }, time.Second, 10*time.Millisecond)
}

// US2 scenario 8, FR-013: the environment opt-out lifts the destination rule for label-declared monitors.
func TestCheckHTTP_OptOutLiftsTheGuard(t *testing.T) {
	t.Setenv(allowLinkLocalEnv, "true")
	_, port, hits := countingHTTP(t)

	result := CheckHTTP(context.Background(), sourced("ep-label-optout", SourceLabel, TypeHTTP, "http://0.0.0.0:"+port+"/"), nil)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, int32(1), hits.Load())
}

// FR-013: a value that is not true keeps the guard on.
func TestCheckHTTP_OptOutFalseKeepsTheGuard(t *testing.T) {
	t.Setenv(allowLinkLocalEnv, "false")
	_, port, hits := countingHTTP(t)

	result := CheckHTTP(context.Background(), sourced("ep-label-optout-false", SourceLabel, TypeHTTP, "http://0.0.0.0:"+port+"/"), nil)

	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, "probe guard")
	assert.Equal(t, int32(0), hits.Load())
}
