// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package eol_test

import (
	"testing"
	"time"

	"github.com/kolapsis/maintenant/internal/eol"
	"github.com/kolapsis/maintenant/internal/update"
)

func TestEvaluateImage(t *testing.T) {
	table, err := eol.LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	today := time.Date(2026, time.November, 20, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		image   string
		state   eol.SupportState
		product string
		cycle   string
	}{
		{"postgres:16.16-alpine3.24", eol.StateSupported, "postgresql", "16"},
		{"docker.io/library/postgres:18.7-alpine3.24", eol.StateSupported, "postgresql", "18"},
		{"postgres:18", eol.StateSupported, "postgresql", "18"},
		{"postgres:14.19", eol.StateEnded, "postgresql", "14"},
		{"mysql:8.0.36", eol.StateEnded, "mysql", "8.0"},
		{"mysql:8", eol.StateUntracked, "", ""},
		{"postgres:latest", eol.StateUntracked, "", ""},
		{"ghcr.io/acme/postgres:18.7", eol.StateUntracked, "", ""},
		{"bitnami/postgresql:18.7", eol.StateUntracked, "", ""},
		{"acme/postgres:16.1", eol.StateUntracked, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.image, func(t *testing.T) {
			repo, tag, _ := update.ParseImageRef(tc.image)
			got := eol.EvaluateImage(repo, tag, table, today)
			if got.State != tc.state || got.Product != tc.product || got.Cycle != tc.cycle {
				t.Fatalf("got %s %s/%s, want %s %s/%s", got.State, got.Product, got.Cycle, tc.state, tc.product, tc.cycle)
			}
			if got.TableSource != table.Source {
				t.Errorf("table source = %q, want %q", got.TableSource, table.Source)
			}
			if tc.state == eol.StateUntracked && (got.SecurityUntil != nil || got.DaysRemaining != nil) {
				t.Errorf("untracked image carries cycle detail: %+v", got)
			}
		})
	}
}

func TestEvaluateImageWarnsBeforeTheEnd(t *testing.T) {
	table, err := eol.LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	got := eol.EvaluateImage("postgres", "14.19", table, time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC))
	if got.State != eol.StateEndingSoon {
		t.Fatalf("state = %s, want %s", got.State, eol.StateEndingSoon)
	}
	if got.SecurityUntil == nil || got.SecurityUntil.String() != "2026-11-12" || got.DaysRemaining == nil || *got.DaysRemaining != 11 {
		t.Errorf("cycle detail = %+v, want security until 2026-11-12 in 11 days", got)
	}
}
