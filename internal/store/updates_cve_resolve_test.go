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

func TestResolveContainerCVEs_ClosesWhatAnEvaluationNoLongerReturns(t *testing.T) {
	us := NewUpdateStore(openTestDB(t))
	ctx := context.Background()
	upsert := func(containerID, cveID string) {
		t.Helper()
		require.NoError(t, us.UpsertContainerCVE(ctx, &update.ContainerCVE{
			ContainerID: containerID, CVEID: cveID, Severity: update.CVESeverityHigh, FirstDetectedAt: time.Now(),
		}))
	}
	active := func(containerID string) []string {
		t.Helper()
		cves, err := us.ListContainerCVEs(ctx, "", containerID)
		require.NoError(t, err)
		ids := []string{}
		for _, c := range cves {
			ids = append(ids, c.CVEID)
		}
		return ids
	}
	upsert("c1", "CVE-A")
	upsert("c1", "CVE-B")
	upsert("c2", "CVE-B")

	require.NoError(t, us.ResolveContainerCVEs(ctx, "", "c1", []string{"CVE-A"}))
	assert.Equal(t, []string{"CVE-A"}, active("c1"))
	assert.Equal(t, []string{"CVE-B"}, active("c2"), "another container keeps its own")

	upsert("c1", "CVE-B")
	assert.ElementsMatch(t, []string{"CVE-A", "CVE-B"}, active("c1"), "a CVE found again is open again")

	require.NoError(t, us.ResolveContainerCVEs(ctx, "", "c1", nil))
	assert.Empty(t, active("c1"))
}
