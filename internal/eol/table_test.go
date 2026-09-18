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
	"encoding/json"
	"testing"
	"time"
)

func TestLoadEmbedded(t *testing.T) {
	table, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	if table.Source != SourceEmbedded {
		t.Errorf("source = %q, want %q", table.Source, SourceEmbedded)
	}
	if table.FetchedAt.IsZero() {
		t.Error("fetched at is zero")
	}
	if len(table.Products) != len(Products) {
		t.Errorf("products = %d, want %d", len(table.Products), len(Products))
	}
	for _, product := range Products {
		if len(table.Products[product]) == 0 {
			t.Errorf("product %s is missing", product)
		}
	}

	debian11, ok := table.Cycle("debian", "11")
	if !ok {
		t.Fatal("debian 11 is missing")
	}
	if debian11.SecurityUntil.String() != "2026-08-31" {
		t.Errorf("debian 11 security until = %s, want 2026-08-31", debian11.SecurityUntil)
	}
	ubuntu2204, ok := table.Cycle("ubuntu", "22.04")
	if !ok {
		t.Fatal("ubuntu 22.04 is missing")
	}
	if ubuntu2204.SecurityUntil.String() != "2027-06-01" {
		t.Errorf("ubuntu 22.04 security until = %s, want 2027-06-01", ubuntu2204.SecurityUntil)
	}
	if !ubuntu2204.LTS {
		t.Error("ubuntu 22.04 is not flagged LTS")
	}
}

func TestValidate(t *testing.T) {
	base, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}

	t.Run("missing product", func(t *testing.T) {
		table := cloneTable(base)
		delete(table.Products, "sles")
		if err := Validate(table); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("duplicate cycle", func(t *testing.T) {
		table := cloneTable(base)
		table.Products["debian"] = append(table.Products["debian"], table.Products["debian"][0])
		if err := Validate(table); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("extended before security", func(t *testing.T) {
		table := cloneTable(base)
		before := NewDate(2000, time.January, 1)
		cycles := table.Products["debian"]
		cycles[0].ExtendedUntil = &before
		if err := Validate(table); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("active after security", func(t *testing.T) {
		table := cloneTable(base)
		after := NewDate(2099, time.January, 1)
		cycles := table.Products["debian"]
		cycles[0].ActiveUntil = &after
		if err := Validate(table); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestDateJSON(t *testing.T) {
	var cycle Cycle
	if err := json.Unmarshal([]byte(`{"name":"11","lts":true,"security_until":"2026-08-31"}`), &cycle); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cycle.SecurityUntil.String() != "2026-08-31" {
		t.Fatalf("security until = %s", cycle.SecurityUntil)
	}
	if cycle.ActiveUntil != nil || cycle.ExtendedUntil != nil {
		t.Fatal("optional dates should stay nil")
	}
	encoded, err := json.Marshal(cycle)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `{"name":"11","lts":true,"security_until":"2026-08-31"}` {
		t.Fatalf("encoded = %s", encoded)
	}
	if err := json.Unmarshal([]byte(`{"security_until":"31/08/2026"}`), &cycle); err == nil {
		t.Fatal("want an error on a malformed date")
	}
}

func TestDateDaysUntil(t *testing.T) {
	date := NewDate(2026, time.August, 31)
	cases := map[string]int{
		"2026-08-31T00:00:00Z": 0,
		"2026-08-31T23:59:59Z": 0,
		"2026-08-01T12:00:00Z": 30,
		"2026-09-01T00:00:01Z": -1,
	}
	for at, want := range cases {
		today, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatalf("parse %q: %v", at, err)
		}
		if got := date.DaysUntil(today); got != want {
			t.Errorf("DaysUntil(%s) = %d, want %d", at, got, want)
		}
	}
}

func cloneTable(src Table) Table {
	out := Table{Source: src.Source, FetchedAt: src.FetchedAt, Products: make(map[string][]Cycle, len(src.Products))}
	for product, cycles := range src.Products {
		out.Products[product] = append([]Cycle(nil), cycles...)
	}
	return out
}
