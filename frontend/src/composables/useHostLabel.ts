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

import { computed } from 'vue'
import { useAgentsStore } from '@/stores/agents'
import { useResourcesStore } from '@/stores/resources'
import { isLocalAgent } from '@/services/apiFetch'

/** Label used for entities reported by the server's own runtime. */
export const LOCAL_HOST_LABEL = 'Local'

/** Multi-host attribution as the API returns it on list rows. */
export interface HostAttributed {
  agent_id?: string | null
  agent_hostname?: string | null
  agent_label?: string | null
}

// Single source of truth for "which host is this row on?", shared by the host
// badge, the container detail panel and the dashboard monitor rows so they all
// name a host the same way — and hide it in the same cases.
export function useHostLabel() {
  const store = useAgentsStore()
  const resources = useResourcesStore()

  // Naming the host only informs when the fleet has more than one and none is
  // already picked as the scope; otherwise it repeats the same value on every
  // row.
  const showHost = computed(() => store.hasRemoteAgents && resources.selected === null)

  function hostLabel(
    agentId?: string | null,
    hostname?: string | null,
    label?: string | null,
  ): string {
    if (isLocalAgent(agentId)) return LOCAL_HOST_LABEL
    if (label) return label
    if (hostname) return hostname
    const agent = store.agents.find((a) => a.agent_id === agentId)
    if (agent) return agent.label || agent.hostname
    return agentId as string
  }

  /** The host to display for a row, or null when it should stay hidden. */
  function hostOf(e: HostAttributed): string | null {
    return showHost.value ? hostLabel(e.agent_id, e.agent_hostname, e.agent_label) : null
  }

  return { showHost, hostLabel, hostOf }
}
