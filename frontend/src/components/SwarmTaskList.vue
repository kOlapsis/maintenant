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
import type { SwarmTaskResponse } from '@/services/swarmApi'
import { timeAgo } from '@/utils/time'

const props = defineProps<{
  tasks: SwarmTaskResponse[]
  groupByNode?: boolean
}>()

const sortedTasks = computed(() => {
  const tasks = [...props.tasks]
  if (props.groupByNode) {
    return tasks.sort((a, b) => {
      if (a.node_id !== b.node_id) return a.node_hostname.localeCompare(b.node_hostname)
      return a.slot - b.slot
    })
  }
  return tasks.sort((a, b) => a.slot - b.slot)
})

const tasksByNode = computed(() => {
  if (!props.groupByNode) return null
  const map = new Map<string, { hostname: string; tasks: SwarmTaskResponse[] }>()
  for (const t of sortedTasks.value) {
    const key = t.node_id || 'unassigned'
    if (!map.has(key)) {
      map.set(key, { hostname: t.node_hostname || 'Unassigned', tasks: [] })
    }
    map.get(key)!.tasks.push(t)
  }
  return map
})

function stateColor(state: string): string {
  switch (state) {
    case 'running': return 'text-pb-status-ok'
    case 'complete': return 'text-slate-400'
    case 'failed': return 'text-red-400'
    case 'rejected': return 'text-red-400'
    case 'shutdown': return 'text-slate-500'
    case 'preparing': case 'starting': case 'assigned': return 'text-sky-400'
    default: return 'text-amber-400'
  }
}

function stateDot(state: string): string {
  switch (state) {
    case 'running': return 'bg-emerald-500'
    case 'failed': case 'rejected': return 'bg-red-500'
    case 'complete': return 'bg-slate-500'
    default: return 'bg-amber-500'
  }
}
</script>

<template>
  <div>
    <div v-if="tasks.length === 0" class="text-sm text-slate-500 py-4 text-center">
      No tasks
    </div>

    <!-- Flat list -->
    <div v-else-if="!groupByNode" class="space-y-1">
      <div
        v-for="task in sortedTasks"
        :key="task.task_id"
        class="bg-pb-surface rounded-lg border border-slate-800 px-3 py-2 flex items-center justify-between"
      >
        <div class="flex items-center gap-2 min-w-0">
          <div :class="['w-2 h-2 rounded-full flex-shrink-0', stateDot(task.state)]" />
          <span class="text-xs text-pb-secondary font-mono">#{{ task.slot }}</span>
          <span :class="['text-xs font-medium', stateColor(task.state)]">{{ task.state }}</span>
          <span v-if="task.node_hostname" class="text-xs text-slate-500 truncate">{{ task.node_hostname }}</span>
        </div>
        <div class="flex items-center gap-3 text-xs text-slate-500 flex-shrink-0 ml-2">
          <span v-if="task.error" class="text-red-400 truncate max-w-48" :title="task.error">{{ task.error }}</span>
          <span v-if="task.exit_code !== null && task.exit_code !== 0" class="text-red-400">exit {{ task.exit_code }}</span>
          <span class="tabular-nums">{{ timeAgo(task.timestamp) }}</span>
        </div>
      </div>
    </div>

    <!-- Grouped by node -->
    <div v-else-if="tasksByNode" class="space-y-3">
      <div v-for="[nodeId, group] in tasksByNode" :key="nodeId">
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">
          {{ group.hostname }} ({{ group.tasks.length }})
        </p>
        <div class="space-y-1">
          <div
            v-for="task in group.tasks"
            :key="task.task_id"
            class="bg-pb-surface rounded-lg border border-slate-800 px-3 py-2 flex items-center justify-between"
          >
            <div class="flex items-center gap-2 min-w-0">
              <div :class="['w-2 h-2 rounded-full flex-shrink-0', stateDot(task.state)]" />
              <span class="text-xs text-pb-secondary font-mono">#{{ task.slot }}</span>
              <span :class="['text-xs font-medium', stateColor(task.state)]">{{ task.state }}</span>
            </div>
            <div class="flex items-center gap-3 text-xs text-slate-500 flex-shrink-0 ml-2">
              <span v-if="task.error" class="text-red-400 truncate max-w-48" :title="task.error">{{ task.error }}</span>
              <span v-if="task.exit_code !== null && task.exit_code !== 0" class="text-red-400">exit {{ task.exit_code }}</span>
              <span class="tabular-nums">{{ timeAgo(task.timestamp) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
