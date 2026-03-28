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
    } catch { /* ignore */ }
  }

  function startListening() {
    sseBus.on('runtime.context_changed', onContextChanged)
  }

  function stopListening() {
    sseBus.off('runtime.context_changed', onContextChanged)
  }

  return {
    context,
    runtime,
    connected,
    label,
    detectedAt,
    metadata,
    loading,
    isDocker,
    isSwarm,
    isKubernetes,
    fetchStatus,
    startListening,
    stopListening,
  }
})
