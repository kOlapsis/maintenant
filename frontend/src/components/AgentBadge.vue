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
import { useAgentsStore } from '@/stores/agents'

const props = defineProps<{
  agentId: string | null | undefined
  // Optional identity provided directly by the API (agent_hostname / agent_label),
  // preferred over the store lookup so the badge works without the agents store loaded.
  hostname?: string | null
  label?: string | null
}>()

const store = useAgentsStore()

const agent = computed(() => {
  if (!props.agentId) return null
  return store.agents.find((a) => a.agent_id === props.agentId) ?? null
})

const displayName = computed(() => {
  if (props.label) return props.label
  if (props.hostname) return props.hostname
  if (agent.value) return agent.value.label || agent.value.hostname
  return props.agentId ?? '—'
})

// Explicit tooltip so the badge reads as a host, and exposes the raw agent id.
const tooltip = computed(() => {
  const parts = [`Hôte : ${displayName.value}`]
  if (props.agentId && props.agentId !== displayName.value) parts.push(props.agentId)
  return parts.join(' · ')
})
</script>

<template>
  <span
    v-if="agentId"
    class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium max-w-[160px]"
    :style="{ backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-secondary)' }"
    :title="tooltip"
  >
    <Server :size="11" class="shrink-0 text-pb-green-500" />
    <span class="truncate">{{ displayName }}</span>
  </span>
</template>
