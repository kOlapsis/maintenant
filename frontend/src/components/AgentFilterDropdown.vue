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
import { ref, computed } from 'vue'
import { useAgentsStore } from '@/stores/agents'

const props = defineProps<{
  modelValue: string | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string | null]
}>()

const store = useAgentsStore()
const open = ref(false)

const activeAgents = computed(() => store.agents.filter((a) => a.status === 'active'))

const selectedLabel = computed(() => {
  if (!props.modelValue) return 'All agents'
  if (props.modelValue === 'local') return 'Local only'
  const found = activeAgents.value.find((a) => a.agent_id === props.modelValue)
  return found ? (found.label || found.hostname) : props.modelValue
})

function select(value: string | null) {
  emit('update:modelValue', value)
  open.value = false
}
</script>

<template>
  <div v-if="activeAgents.length > 0" class="relative">
    <button
      class="flex items-center gap-1.5 rounded-lg border border-slate-700 bg-[#0B0E13] px-3 py-1.5 text-xs text-slate-300 hover:border-slate-600 transition-colors"
      @click="open = !open"
    >
      <span>{{ selectedLabel }}</span>
      <svg class="w-3 h-3 text-slate-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
      </svg>
    </button>

    <div
      v-if="open"
      class="absolute right-0 mt-1 z-50 min-w-[180px] rounded-xl border border-slate-700 bg-[#12151C] shadow-2xl py-1"
    >
      <button
        class="w-full text-left px-3 py-2 text-xs hover:bg-slate-800/50 transition-colors"
        :class="modelValue === null ? 'text-white font-medium' : 'text-slate-400'"
        @click="select(null)"
      >
        All agents
      </button>
      <button
        class="w-full text-left px-3 py-2 text-xs hover:bg-slate-800/50 transition-colors"
        :class="modelValue === 'local' ? 'text-white font-medium' : 'text-slate-400'"
        @click="select('local')"
      >
        Local only
      </button>
      <div v-if="activeAgents.length > 0" class="my-1 border-t border-slate-800" />
      <button
        v-for="agent in activeAgents"
        :key="agent.agent_id"
        class="w-full text-left px-3 py-2 text-xs hover:bg-slate-800/50 transition-colors flex items-center gap-2"
        :class="modelValue === agent.agent_id ? 'text-white font-medium' : 'text-slate-400'"
        @click="select(agent.agent_id)"
      >
        <span
          class="w-1.5 h-1.5 rounded-full shrink-0"
          :class="agent.connection_state === 'connected' ? 'bg-pb-green-500' : 'bg-slate-600'"
        />
        {{ agent.label || agent.hostname }}
      </button>
    </div>

    <div
      v-if="open"
      class="fixed inset-0 z-40"
      @click="open = false"
    />
  </div>
</template>
