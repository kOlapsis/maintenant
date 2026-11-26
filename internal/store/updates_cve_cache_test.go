// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/update"
)

func TestReplaceCVECache_HoldsExactlyTheLastAnswer(t *testing.T) {
	us := NewUpdateStore(openTestDB(t))
	ctx := context.Background()
	const eco, pkg, version = "Go", "github.com/traefik/traefik/v3", "3.1.0"
	entry := func(id string, expires time.Time) *update.CVECacheEntry {
		return &update.CVECacheEntry{
			Ecosystem: eco, PackageName: pkg, PackageVersion: version, CVEID: id,
			Severity: update.CVESeverityHigh, FetchedAt: time.Now(), ExpiresAt: expires,
		}
	}
	cached := func() []string {
		t.Helper()
		entries, err := us.GetCVECacheEntries(ctx, eco, pkg, version)
		require.NoError(t, err)
		ids := []string{}
		for _, e := range entries {
			ids = append(ids, e.CVEID)
		}
		return ids
	}

	require.NoError(t, us.ReplaceCVECache(ctx, eco, pkg, version, []*update.CVECacheEntry{
		entry("CVE-A", time.Now().Add(-time.Hour)), entry("CVE-B", time.Now().Add(-time.Hour)),
	}))
	assert.ElementsMatch(t, []string{"CVE-A", "CVE-B"}, cached())

	require.NoError(t, us.ReplaceCVECache(ctx, eco, pkg, version, []*update.CVECacheEntry{entry("CVE-B", time.Now().Add(time.Hour))}))
	assert.Equal(t, []string{"CVE-B"}, cached(), "an advisory the last answer no longer lists leaves the cache")

	other := entry("CVE-Z", time.Now().Add(time.Hour))
	other.PackageVersion = "3.2.0"
	require.NoError(t, us.ReplaceCVECache(ctx, eco, pkg, "3.2.0", []*update.CVECacheEntry{other}))
	assert.Equal(t, []string{"CVE-B"}, cached(), "another version has its own cache")
}
