<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import { computed } from 'vue'
import { Server } from 'lucide-vue-next'
import { isLocalAgent } from '@/services/apiFetch'
import { useHostLabel } from '@/composables/useHostLabel'

const props = withDefaults(
  defineProps<{
    agentId: string | null | undefined
    // Optional identity provided directly by the API (agent_hostname / agent_label),
    // preferred over the store lookup so the badge works without the agents store loaded.
    hostname?: string | null
    label?: string | null
    // Name the server's own runtime too, as "Local". Off by default: on a
    // single-host install the badge would be the same on every row.
    showLocal?: boolean
  }>(),
  { showLocal: false },
)

const { hostLabel } = useHostLabel()

const isLocal = computed(() => isLocalAgent(props.agentId))
const visible = computed(() => !isLocal.value || props.showLocal)

const displayName = computed(() => hostLabel(props.agentId, props.hostname, props.label))

// Explicit tooltip so the badge reads as a host, and exposes the raw agent id.
const tooltip = computed(() => {
  const parts = [`Host: ${displayName.value}`]
  if (props.agentId && props.agentId !== displayName.value) parts.push(props.agentId)
  return parts.join(' · ')
})
</script>

<template>
  <span
    v-if="visible"
    class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium max-w-[160px]"
    :style="{ backgroundColor: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-secondary)' }"
    :title="tooltip"
  >
    <Server :size="11" class="shrink-0" :class="isLocal ? 'text-mnt-muted' : 'text-mnt-green-500'" />
    <span class="truncate">{{ displayName }}</span>
  </span>
</template>
