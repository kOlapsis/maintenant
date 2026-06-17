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

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  fetchRuntimeStatus,
  type RuntimeContextValue,
  type RuntimeValue,
  type SwarmMetadata,
  type KubernetesMetadata,
  type DockerMetadata,
} from '@/services/runtimeApi'
import { sseBus } from '@/services/sseBus'
import { showToast } from '@/composables/useToast'

function contextToLabel(context: RuntimeContextValue): string {
  if (context === 'swarm') return 'Services'
  if (context === 'kubernetes') return 'Workloads'
  return 'Containers'
}

function contextToRuntime(context: RuntimeContextValue): RuntimeValue {
  if (context === 'kubernetes') return 'kubernetes'
  return 'docker'
}

export const useRuntimeStore = defineStore('runtime', () => {
  const context = ref<RuntimeContextValue>('docker')
  const runtime = ref<RuntimeValue>('docker')
  const connected = ref(true)
  const label = ref('Containers')
  const detectedAt = ref<string | null>(null)
  const metadata = ref<SwarmMetadata | KubernetesMetadata | DockerMetadata>({})
  const loading = ref(false)
  // false until the first successful status fetch — runtime-specific nav waits
  // for the real runtime instead of flashing the optimistic 'docker' default.
  const loaded = ref(false)

  const isDocker = computed(() => context.value === 'docker')
  const isSwarm = computed(() => context.value === 'swarm')
  const isKubernetes = computed(() => context.value === 'kubernetes')

  async function fetchStatus() {
    loading.value = true
    try {
      const status = await fetchRuntimeStatus()
      context.value = status.context
      runtime.value = status.runtime
      connected.value = status.connected
      label.value = status.label
      detectedAt.value = status.detected_at
      metadata.value = status.metadata
      loaded.value = true
    } finally {
      loading.value = false
    }
  }

  function onContextChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as {
        previous: RuntimeContextValue
        current: RuntimeContextValue
        message: string
        detected_at: string
      }
      context.value = data.current
      runtime.value = contextToRuntime(data.current)
      label.value = contextToLabel(data.current)
      detectedAt.value = data.detected_at
      if (data.message) {
        showToast(data.message, 'info')
      }
      // Refresh full status so metadata (incl. service_count) stays authoritative.
      void fetchStatus()
    } catch { /* ignore */ }
  }

  // Service deploy/remove changes the Swarm service count, which decides whether
  // the Docker "Containers" view stays visible — refetch so the nav reacts live.
  function onServiceCountChanged() {
    void fetchStatus()
  }

  function onAvailabilityChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { name: string; connected: boolean }
      connected.value = data.connected
    } catch { /* ignore */ }
  }

  function startListening() {
    sseBus.on('runtime.context_changed', onContextChanged)
    sseBus.on('runtime.availability_changed', onAvailabilityChanged)
    sseBus.on('swarm.service_discovered', onServiceCountChanged)
    sseBus.on('swarm.service_removed', onServiceCountChanged)
  }

  function stopListening() {
    sseBus.off('runtime.context_changed', onContextChanged)
    sseBus.off('runtime.availability_changed', onAvailabilityChanged)
    sseBus.off('swarm.service_discovered', onServiceCountChanged)
    sseBus.off('swarm.service_removed', onServiceCountChanged)
  }

  return {
    context,
    runtime,
    connected,
    label,
    detectedAt,
    metadata,
    loading,
    loaded,
    isDocker,
    isSwarm,
    isKubernetes,
    fetchStatus,
    startListening,
    stopListening,
  }
})
