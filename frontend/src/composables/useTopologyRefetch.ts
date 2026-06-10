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

import { onMounted, onUnmounted } from 'vue'
import { sseBus } from '@/services/sseBus'
import { useResourcesStore } from '@/stores/resources'

// useTopologyRefetch reloads a per-agent view when the server emits a
// "<runtime>.topology_changed" event for an agent that matches the current host
// scope. The "local" scope is driven by the runtime's own informer events, so it
// is ignored here. Auto-registers on mount and cleans up on unmount.
export function useTopologyRefetch(eventType: string, reload: () => void) {
  const resources = useResourcesStore()

  function handler(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { agent_id: string }
      const sel = resources.selected
      if (sel === 'local') return
      if (sel !== null && sel !== data.agent_id) return
      reload()
    } catch {
      /* ignore malformed payloads */
    }
  }

  onMounted(() => sseBus.on(eventType, handler))
  onUnmounted(() => sseBus.off(eventType, handler))
}
