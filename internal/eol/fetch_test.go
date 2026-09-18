// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package eol

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func fixtureServer(t *testing.T, override map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		product := path.Base(r.URL.Path)
		if handler, ok := override[product]; ok {
			handler(w, r)
			return
		}
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("product %s: headers = %v", product, r.Header)
		}
		body, err := os.ReadFile("testdata/" + product + ".json")
		if err != nil {
			http.Error(w, "unknown product", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetch(t *testing.T) {
	server := fixtureServer(t, nil)
	fetched := time.Date(2026, time.September, 18, 9, 0, 0, 0, time.UTC)
	fetcher := &Fetcher{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return fetched }}

	table, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if err := Validate(table); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if table.Source != SourceRemote {
		t.Errorf("source = %q, want %q", table.Source, SourceRemote)
	}
	if !table.FetchedAt.Equal(fetched) {
		t.Errorf("fetched at = %s, want %s", table.FetchedAt, fetched)
	}

	debian11, ok := table.Cycle("debian", "11")
	if !ok {
		t.Fatal("debian 11 is missing")
	}
	if debian11.SecurityUntil.String() != "2026-08-31" {
		t.Errorf("debian 11 security until = %s, want 2026-08-31", debian11.SecurityUntil)
	}

	alpine324, ok := table.Cycle("alpine-linux", "3.24")
	if !ok {
		t.Fatal("alpine 3.24 is missing")
	}
	if alpine324.ActiveUntil != nil || alpine324.ExtendedUntil != nil {
		t.Errorf("alpine 3.24 optional dates = %v / %v, want nil", alpine324.ActiveUntil, alpine324.ExtendedUntil)
	}
}

func TestFetchRejectsPartialTable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"not found": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html><body>404</body></html>"))
		},
		"release without eolFrom": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"releases":[{"name":"16.0","isLts":false,"eolFrom":null}]}}`))
		},
		"malformed date": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"releases":[{"name":"16.0","eolFrom":"31/08/2026"}]}}`))
		},
		"no release": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"releases":[]}}`))
		},
	}

	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			server := fixtureServer(t, map[string]http.HandlerFunc{"sles": handler})
			fetcher := &Fetcher{BaseURL: server.URL, Client: server.Client()}

			table, err := fetcher.Fetch(context.Background())
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "sles") {
				t.Errorf("error = %v, want the faulty product named", err)
			}
			if len(table.Products) != 0 {
				t.Errorf("table = %+v, want none", table)
			}
		})
	}
}

// TestWriteEmbeddedTable regenerates table.json from endoflife.date; run it with `make eol-table`.
func TestWriteEmbeddedTable(t *testing.T) {
	if os.Getenv("MAINTENANT_EOL_WRITE_TABLE") != "1" {
		t.Skip("set MAINTENANT_EOL_WRITE_TABLE=1 to regenerate table.json")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fetcher := &Fetcher{}
	table, err := fetcher.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	table.Source = SourceEmbedded
	table.FetchedAt = table.FetchedAt.UTC().Truncate(24 * time.Hour)

	encoded, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		t.Fatalf("encode table: %v", err)
	}
	if err := os.WriteFile("table.json", append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write table.json: %v", err)
	}
	t.Logf("wrote table.json with %d products", len(table.Products))
}
