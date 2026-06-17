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
import type { SwarmMetadata } from '@/services/runtimeApi'

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

  // A local Swarm manager with zero deployed services still runs plain
  // containers, so keep the Docker "Containers" view reachable alongside the
  // (empty) Swarm views until a service is actually deployed.
  const localSwarmNoServices = computed(() => {
    if (runtimeStore.context !== 'swarm') return false
    const meta = runtimeStore.metadata as Partial<SwarmMetadata>
    return (meta.service_count ?? 0) === 0
  })

  const availableRuntimes = computed<string[]>(() => {
    const sel = resources.selected
    if (sel === 'local') {
      const set = new Set<string>([localRuntime.value])
      if (localSwarmNoServices.value) set.add('docker')
      return [...set]
    }
    if (sel) {
      const agent = activeAgents.value.find((a) => a.agent_id === sel)
      return agent ? [agent.detected_runtime] : []
    }
    // "All resources": union of the local runtime and every active agent's.
    const set = new Set<string>([localRuntime.value])
    for (const a of activeAgents.value) set.add(a.detected_runtime)
    if (localSwarmNoServices.value) set.add('docker')
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
