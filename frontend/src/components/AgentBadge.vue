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
import { useAgentsStore } from '@/stores/agents'

const props = defineProps<{
  agentId: string | null | undefined
}>()

const store = useAgentsStore()

const agent = computed(() => {
  if (!props.agentId) return null
  return store.agents.find((a) => a.agent_id === props.agentId) ?? null
})

const label = computed(() => {
  if (!agent.value) return props.agentId ?? '—'
  return agent.value.label || agent.value.hostname
})
</script>

<template>
  <span
    v-if="agentId"
    class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
    :style="{ backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-secondary)' }"
    :title="agentId"
  >
    <span class="w-1.5 h-1.5 rounded-full bg-pb-green-500 shrink-0" />
    {{ label }}
  </span>
</template>
