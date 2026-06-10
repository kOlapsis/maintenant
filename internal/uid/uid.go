// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

// Package uid is the single source of truth for entity identifiers.
//
// Every entity uses a UUID primary key so that ids can be minted independently
// by the server or by a remote agent without coordination. There are two ways
// to obtain an id:
//
//   - New() mints a fresh time-ordered UUIDv7, for server-created entities and
//     for telemetry events that have no stable natural key.
//   - Derive() computes a deterministic UUIDv5 from a natural key, so the agent
//     and the server independently arrive at the same id for the same entity.
package uid

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// LocalAgent is the sentinel agent id for the server's own in-process runtime
// (as opposed to a remote enrolled agent). Locally discovered entities are
// attributed to it so that agent_id is never NULL and the (agent_id, natural
// key) identity is uniform across local and remote sources.
const LocalAgent = "00000000-0000-0000-0000-000000000000"

// unitSep joins natural-key parts unambiguously so that ("ab","c") and ("a","bc")
// derive distinct ids.
const unitSep = "\x1f"

// nsRoot is the fixed root namespace for all deterministic ids; its bytes spell
// "maintenant". It MUST NOT change — every derived id depends on it.
var nsRoot = uuid.MustParse("6d61696e-7465-6e61-6e74-000000000001")

// Per-entity namespaces, derived once from nsRoot and stable forever. Changing a
// namespace would orphan every id previously derived under it.
var (
	nsContainer    = uuid.NewSHA1(nsRoot, []byte("container"))
	nsEndpoint     = uuid.NewSHA1(nsRoot, []byte("endpoint"))
	nsCert         = uuid.NewSHA1(nsRoot, []byte("cert_monitor"))
	nsSwarmNode    = uuid.NewSHA1(nsRoot, []byte("swarm_node"))
	nsSwarmService = uuid.NewSHA1(nsRoot, []byte("swarm_service"))
	nsSwarmTask    = uuid.NewSHA1(nsRoot, []byte("swarm_task"))
	nsK8sPod       = uuid.NewSHA1(nsRoot, []byte("k8s_pod"))
	nsK8sNode      = uuid.NewSHA1(nsRoot, []byte("k8s_node"))
	nsK8sWorkload  = uuid.NewSHA1(nsRoot, []byte("k8s_workload"))
	nsNamespace    = uuid.NewSHA1(nsRoot, []byte("k8s_namespace"))
)

// New returns a fresh time-ordered UUIDv7 string.
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Derive returns a deterministic UUIDv5 string from a namespace and the parts of
// a natural key. The same inputs always yield the same id. Parts are trimmed and
// joined with a unit separator; callers normalize case per field (e.g. lowercase
// hostnames) before calling.
func Derive(ns uuid.UUID, parts ...string) string {
	trimmed := make([]string, len(parts))
	for i, p := range parts {
		trimmed[i] = strings.TrimSpace(p)
	}
	return uuid.NewSHA1(ns, []byte(strings.Join(trimmed, unitSep))).String()
}

// Container derives a container id from the reporting agent and the runtime's
// external container id (Docker SHA / pod uid). Pass LocalAgent for the server's
// own runtime.
func Container(agentID, externalID string) string {
	return Derive(nsContainer, agentID, externalID)
}

// EndpointLabel derives a label-sourced endpoint id from the agent, the owning
// container name and the maintenant label key.
func EndpointLabel(agentID, containerName, labelKey string) string {
	return Derive(nsEndpoint, agentID, containerName, labelKey)
}

// CertMonitor derives a cert monitor id from the agent and the monitored
// hostname:port.
func CertMonitor(agentID, hostname string, port int) string {
	return Derive(nsCert, agentID, hostname, strconv.Itoa(port))
}

// SwarmNode derives a swarm node id from the agent and the runtime node id.
func SwarmNode(agentID, nodeID string) string {
	return Derive(nsSwarmNode, agentID, nodeID)
}

// SwarmService derives a swarm service id from the agent and the runtime service id.
func SwarmService(agentID, serviceID string) string {
	return Derive(nsSwarmService, agentID, serviceID)
}

// SwarmTask derives a swarm task id from the agent and the runtime task id.
func SwarmTask(agentID, taskID string) string {
	return Derive(nsSwarmTask, agentID, taskID)
}

// Pod derives a Kubernetes pod id from the agent, namespace and pod name.
func Pod(agentID, namespace, name string) string {
	return Derive(nsK8sPod, agentID, namespace, name)
}

// K8sNode derives a Kubernetes node id from the agent and the node name.
func K8sNode(agentID, name string) string {
	return Derive(nsK8sNode, agentID, name)
}

// K8sWorkload derives a Kubernetes workload id from the agent and the workload's
// natural id ("{namespace}/{kind}/{name}").
func K8sWorkload(agentID, workloadID string) string {
	return Derive(nsK8sWorkload, agentID, workloadID)
}

// Namespace derives a Kubernetes namespace id from the agent and the namespace name.
func Namespace(agentID, name string) string {
	return Derive(nsNamespace, agentID, name)
}

// Agent returns agentID, or LocalAgent when it is empty. Eases the transition
// from the old nullable agent_id where NULL/"" meant the local runtime.
func Agent(agentID string) string {
	if strings.TrimSpace(agentID) == "" {
		return LocalAgent
	}
	return agentID
}
