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
import { computed, onMounted, onUnmounted } from 'vue'
import { useSwarmStore } from '@/stores/swarm'
import { timeAgo } from '@/utils/time'

const emit = defineEmits<{
  select: [nodeId: string]
}>()

const swarmStore = useSwarmStore()

onMounted(() => {
  swarmStore.loadNodes()
  swarmStore.startListening()
})

onUnmounted(() => {
  swarmStore.stopListening()
})

const sortedNodes = computed(() => {
  return [...swarmStore.nodes].sort((a, b) => {
    if (a.role !== b.role) return a.role === 'manager' ? -1 : 1
    return a.hostname.localeCompare(b.hostname)
  })
})

function statusColor(status: string): string {
  switch (status) {
    case 'ready': return 'bg-emerald-500'
    case 'down': return 'bg-red-500'
    case 'disconnected': return 'bg-amber-500'
    default: return 'bg-slate-500'
  }
}

function statusText(status: string): string {
  switch (status) {
    case 'ready': return 'text-emerald-400'
    case 'down': return 'text-red-400'
    case 'disconnected': return 'text-amber-400'
    default: return 'text-slate-400'
  }
}

function availabilityStyle(availability: string): string {
  switch (availability) {
    case 'active': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
    case 'drain': return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
    case 'pause': return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
    default: return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
  }
}
</script>

<template>
  <div>
    <div class="mb-3 flex items-center justify-between">
      <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Cluster Nodes</p>
      <div class="flex items-center gap-3 text-xs text-slate-400">
        <span>{{ swarmStore.managerCount }} managers</span>
        <span class="text-slate-600">|</span>
        <span>{{ swarmStore.workerCount }} workers</span>
        <span class="text-slate-600">|</span>
        <span :class="swarmStore.readyCount === swarmStore.nodes.length ? 'text-emerald-400' : 'text-amber-400'">
          {{ swarmStore.readyCount }}/{{ swarmStore.nodes.length }} ready
        </span>
      </div>
    </div>

    <div v-if="swarmStore.loading && swarmStore.nodes.length === 0" class="text-sm text-slate-500 py-8 text-center">
      Loading nodes...
    </div>

    <div v-else-if="swarmStore.error" class="text-sm text-red-400 py-8 text-center">
      {{ swarmStore.error }}
    </div>

    <div v-else-if="swarmStore.nodes.length === 0" class="text-sm text-slate-500 py-8 text-center">
      No nodes found
    </div>

    <div v-else class="space-y-1">
      <div
        v-for="node in sortedNodes"
        :key="node.node_id"
        class="bg-pb-surface rounded-xl border border-slate-800 px-4 py-3 hover:bg-slate-800/25 transition-all cursor-pointer group"
        @click="emit('select', node.node_id)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3 min-w-0">
            <!-- Status dot -->
            <div class="relative flex-shrink-0">
              <div :class="['w-2.5 h-2.5 rounded-full', statusColor(node.status)]" />
              <div
                v-if="node.status === 'ready'"
                :class="['absolute inset-0 w-2.5 h-2.5 rounded-full animate-ping opacity-30', statusColor(node.status)]"
              />
            </div>

            <!-- Hostname -->
            <span class="text-sm text-pb-primary font-medium truncate">{{ node.hostname }}</span>

            <!-- Role badge -->
            <span
              :class="[
                'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border',
                node.role === 'manager'
                  ? 'text-violet-400 bg-violet-400/10 border-violet-400/20'
                  : 'text-slate-400 bg-slate-400/10 border-slate-400/20',
              ]"
            >
              {{ node.role }}
            </span>

            <!-- Availability badge -->
            <span
              v-if="node.availability !== 'active'"
              :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', availabilityStyle(node.availability)]"
            >
              {{ node.availability }}
            </span>
          </div>

          <div class="flex items-center gap-4 text-xs text-slate-400 flex-shrink-0 ml-4">
            <!-- Status text -->
            <span :class="['font-medium', statusText(node.status)]">{{ node.status }}</span>

            <!-- Task count -->
            <span class="tabular-nums">{{ node.task_count }} tasks</span>

            <!-- Engine version -->
            <span v-if="node.engine_version" class="text-slate-500 hidden sm:inline">
              v{{ node.engine_version }}
            </span>

            <!-- Last seen -->
            <span class="text-slate-500 tabular-nums hidden md:inline" :title="node.last_seen_at">
              {{ timeAgo(node.last_seen_at) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
