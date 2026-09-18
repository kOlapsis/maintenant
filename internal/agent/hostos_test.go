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

package agent

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agentpb"
	"github.com/kolapsis/maintenant/internal/hoststat"
	"github.com/kolapsis/maintenant/internal/kubernetes"
)

// hostOSReads hands out one OSRelease per call, repeating the last one.
type hostOSReads struct {
	mu   sync.Mutex
	next []hoststat.OSRelease
	last hoststat.OSRelease
	n    int
}

func (r *hostOSReads) read() hoststat.OSRelease {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	if len(r.next) > 0 {
		r.last = r.next[0]
		r.next = r.next[1:]
	}
	return r.last
}

func (r *hostOSReads) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}

func startHostOSStream(t *testing.T, reads *hostOSReads) (*captureSink, func()) {
	t.Helper()

	prev := hostOSInterval
	hostOSInterval = 5 * time.Millisecond
	t.Cleanup(func() { hostOSInterval = prev })

	sink := &captureSink{}
	spool := NewSpool(t.TempDir(), SpoolConfig{}, testLogger())
	spool.Attach(sink)
	t.Cleanup(func() { _ = spool.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- streamHostOS(ctx, &Identity{AgentID: "agent-A"}, reads.read, spool, testLogger()) }()

	return sink, func() {
		cancel()
		require.NoError(t, <-done)
	}
}

func hostOSBodies(sink *captureSink) []*agentpb.HostOSMsg {
	var out []*agentpb.HostOSMsg
	for _, ev := range sink.events() {
		if body := ev.GetHostOs(); body != nil {
			out = append(out, body)
		}
	}
	return out
}

func TestStreamHostOS_SendsImmediatelyThenOnlyOnChange(t *testing.T) {
	debian := hoststat.OSRelease{ID: "debian", VersionID: "12", PrettyName: "Debian GNU/Linux 12 (bookworm)", Source: hoststat.OSSourceHostFile}
	reads := &hostOSReads{next: []hoststat.OSRelease{debian}}

	sink, stop := startHostOSStream(t, reads)

	require.Eventually(t, func() bool { return len(hostOSBodies(sink)) == 1 }, time.Second, 2*time.Millisecond,
		"the identity must be reported as soon as the collector starts")

	require.Eventually(t, func() bool { return reads.calls() >= 4 }, time.Second, 2*time.Millisecond)
	assert.Len(t, hostOSBodies(sink), 1, "an unchanged re-read must not be reported again")

	reads.mu.Lock()
	reads.next = []hoststat.OSRelease{{ID: "debian", VersionID: "13", PrettyName: "Debian GNU/Linux 13 (trixie)", Source: hoststat.OSSourceHostFile}}
	reads.mu.Unlock()

	require.Eventually(t, func() bool { return len(hostOSBodies(sink)) == 2 }, time.Second, 2*time.Millisecond,
		"an upgrade must be reported without a restart")
	stop()

	bodies := hostOSBodies(sink)
	require.Len(t, bodies, 2)
	assert.Equal(t, "12", bodies[0].GetVersionId())
	assert.Equal(t, "13", bodies[1].GetVersionId())
	assert.Equal(t, agentpb.HostOSSource_HOST_OS_SOURCE_HOST_FILE, bodies[1].GetSource())
}

func TestStreamHostOS_ReportsUnavailableIdentity(t *testing.T) {
	reads := &hostOSReads{next: []hoststat.OSRelease{{UnavailableReason: hoststat.OSReasonMountMissing}}}

	sink, stop := startHostOSStream(t, reads)
	require.Eventually(t, func() bool { return len(hostOSBodies(sink)) == 1 }, time.Second, 2*time.Millisecond)
	stop()

	body := hostOSBodies(sink)[0]
	assert.Equal(t, hoststat.OSReasonMountMissing, body.GetUnavailableReason())
	assert.Equal(t, agentpb.HostOSSource_HOST_OS_SOURCE_UNSPECIFIED, body.GetSource())
	assert.Empty(t, body.GetId())
}

func TestHostOSEvent_ConvertsSource(t *testing.T) {
	cases := map[string]agentpb.HostOSSource{
		hoststat.OSSourceHostFile:       agentpb.HostOSSource_HOST_OS_SOURCE_HOST_FILE,
		hoststat.OSSourceKubernetesNode: agentpb.HostOSSource_HOST_OS_SOURCE_KUBERNETES_NODE,
		"":                              agentpb.HostOSSource_HOST_OS_SOURCE_UNSPECIFIED,
	}
	for source, want := range cases {
		ev := hostOSEvent(&Identity{AgentID: "agent-A"}, hoststat.OSRelease{ID: "debian", Source: source})
		require.Equal(t, "agent-A", ev.GetAgentId())
		require.NotEmpty(t, ev.GetEventId())
		require.NotNil(t, ev.GetObservedAt())
		assert.Equal(t, want, ev.GetHostOs().GetSource(), "source %q", source)
	}
}

// nodeSnapshotSource is a kubernetes.SnapshotSource holding a fixed set of pods
// and nodes, enough to resolve the node the agent runs on.
type nodeSnapshotSource struct {
	pods  []kubernetes.K8sPod
	nodes []kubernetes.K8sNode
}

func (s nodeSnapshotSource) ListNamespaces(context.Context) ([]string, error) { return nil, nil }
func (s nodeSnapshotSource) ListWorkloads(context.Context, []string) ([]kubernetes.K8sWorkloadGroup, error) {
	return nil, nil
}
func (s nodeSnapshotSource) ListPods(context.Context, []string, kubernetes.PodFilters) ([]kubernetes.K8sPod, error) {
	return s.pods, nil
}
func (s nodeSnapshotSource) ListNodes(context.Context) ([]kubernetes.K8sNode, error) {
	return s.nodes, nil
}
func (s nodeSnapshotSource) ListAllEvents(context.Context) ([]kubernetes.K8sEventRef, error) {
	return nil, nil
}

func TestNodeOSRelease_DerivesIdentityFromTheAgentNode(t *testing.T) {
	hostname, err := os.Hostname()
	require.NoError(t, err)

	src := nodeSnapshotSource{
		pods: []kubernetes.K8sPod{
			{Name: "other", Namespace: "default", NodeName: "n2"},
			{Name: hostname, Namespace: "maintenant", NodeName: "n1"},
		},
		nodes: []kubernetes.K8sNode{
			{Name: "n1", OSImage: "Ubuntu 22.04.4 LTS"},
			{Name: "n2", OSImage: "Debian GNU/Linux 12 (bookworm)"},
		},
	}

	rel := nodeOSRelease(context.Background(), src, "")
	assert.Equal(t, "ubuntu", rel.ID)
	assert.Equal(t, "22.04", rel.VersionID)
	assert.Equal(t, "Ubuntu 22.04.4 LTS", rel.PrettyName)
	assert.Equal(t, hoststat.OSSourceKubernetesNode, rel.Source)
	assert.Empty(t, rel.UnavailableReason)

	forced := nodeOSRelease(context.Background(), src, "n2")
	assert.Equal(t, "debian", forced.ID)
	assert.Equal(t, "12", forced.VersionID)
}

func TestNodeOSRelease_NodeNotFound(t *testing.T) {
	src := nodeSnapshotSource{
		pods:  []kubernetes.K8sPod{{Name: "someone-else", NodeName: "n1"}},
		nodes: []kubernetes.K8sNode{{Name: "n1", OSImage: "Ubuntu 22.04.4 LTS"}},
	}

	rel := nodeOSRelease(context.Background(), src, "")
	assert.Equal(t, hoststat.OSReasonNodeNotFound, rel.UnavailableReason)
	assert.Equal(t, hoststat.OSSourceKubernetesNode, rel.Source)
	assert.Empty(t, rel.ID)

	missing := nodeOSRelease(context.Background(), src, "n9")
	assert.Equal(t, hoststat.OSReasonNodeNotFound, missing.UnavailableReason)
}
