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

import { computed } from 'vue'
import { useRuntimeStore } from '@/stores/runtime'
import { useAgentsStore } from '@/stores/agents'
import { useResourcesStore } from '@/stores/resources'

// useFleetRuntimes derives which container runtimes are reachable for the
// currently selected host scope, so the navigation can show the matching views.
//
// The fleet is the local server runtime (its real runtime, exposed by the
// runtime store — the LocalAgent row's detected_runtime is just the "local"
// sentinel) plus every active agent's detected_runtime. The selection narrows
// it: "all" → the union, "local" → the server runtime, an agent → that agent's
// runtime.
export function useFleetRuntimes() {
  const runtimeStore = useRuntimeStore()
  const agentsStore = useAgentsStore()
  const resources = useResourcesStore()

  const activeAgents = computed(() => agentsStore.agents.filter((a) => a.status === 'active'))

  // The server's own runtime: 'docker' | 'swarm' | 'kubernetes'.
  const localRuntime = computed(() => runtimeStore.context)

  // Swarm mode is an overlay on the Docker runtime, not a replacement: the host
  // keeps running plain containers (the tasks themselves, plus anything started
  // outside a service), and the Docker views keep working on it. So a Swarm
  // scope offers 'docker' as well, whatever the deployed-service count — gating
  // that on service_count == 0 hid the Containers entry on every real cluster
  // while the page itself still worked by direct URL (issue #28).
  const expand = (rt: string): string[] => (rt === 'swarm' ? ['swarm', 'docker'] : [rt])

  // The local server's runtimes, gated on the runtime status being known. Until
  // the first fetch resolves we contribute nothing, so runtime-specific nav
  // (Containers/Services/Tasks/Workloads/Pods) waits instead of flashing the
  // optimistic 'docker' default.
  const localRuntimes = (): string[] => {
    if (!runtimeStore.loaded) return []
    return expand(localRuntime.value)
  }

  const availableRuntimes = computed<string[]>(() => {
    const sel = resources.selected
    if (sel === 'local') return localRuntimes()
    if (sel) {
      const agent = activeAgents.value.find((a) => a.agent_id === sel)
      return agent ? expand(agent.detected_runtime) : []
    }
    // "All resources": union of the local runtime and every active agent's.
    const set = new Set<string>(localRuntimes())
    for (const a of activeAgents.value) {
      for (const rt of expand(a.detected_runtime)) set.add(rt)
    }
    return [...set]
  })

  // True when every runtime in the selected scope is Kubernetes — the classic
  // dashboard (Docker monitors) has nothing to show, the cluster overview does.
  const kubernetesOnly = computed(
    () =>
      availableRuntimes.value.length > 0 &&
      availableRuntimes.value.every((rt) => rt === 'kubernetes'),
  )

  return { availableRuntimes, localRuntime, kubernetesOnly }
}
