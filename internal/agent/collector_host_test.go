// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/hoststat"
)

func TestHostResourceEvent_Structure(t *testing.T) {
	reader := hoststat.NewReader()
	ev := hostResourceEvent("agent-x", reader)

	require.Equal(t, "agent-x", ev.GetAgentId())
	assert.NotEmpty(t, ev.GetEventId())

	res := ev.GetResource()
	require.NotNil(t, res, "host event must carry a ResourceSample")
	assert.Empty(t, res.GetContainerId(), "host samples carry an empty container_id")
	assert.Positive(t, res.GetHostDiskTotalBytes(), "disk total comes from statfs")
	assert.LessOrEqual(t, res.GetHostDiskUsedBytes(), res.GetHostDiskTotalBytes())
}

func TestHostResourceEvent_ReportsMemoryAfterSampling(t *testing.T) {
	reader := hoststat.NewReader()
	go reader.Start(t.Context())

	require.Eventually(t, func() bool { return reader.MemTotal() > 0 },
		3*time.Second, 50*time.Millisecond, "reader should read /proc/meminfo")

	res := hostResourceEvent("agent-x", reader).GetResource()
	assert.Positive(t, res.GetMemoryLimitBytes(), "host mem total should be reported")
	assert.GreaterOrEqual(t, res.GetMemoryLimitBytes(), res.GetMemoryBytes())
}
