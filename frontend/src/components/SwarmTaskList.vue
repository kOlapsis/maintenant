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
    case 'running': return 'text-mnt-status-ok'
    case 'complete': return 'text-mnt-muted'
    case 'failed': return 'text-mnt-status-down'
    case 'rejected': return 'text-mnt-status-down'
    case 'shutdown': return 'text-mnt-muted'
    case 'preparing': case 'starting': case 'assigned': return 'text-mnt-secondary'
    default: return 'text-mnt-status-warn'
  }
}

function stateDot(state: string): string {
  switch (state) {
    case 'running': return 'bg-mnt-sev-ok-solid'
    case 'failed': case 'rejected': return 'bg-mnt-sev-incident-solid'
    case 'complete': return 'bg-mnt-sev-neutral-solid'
    default: return 'bg-mnt-sev-warning-solid'
  }
}
</script>

<template>
  <div>
    <div v-if="tasks.length === 0" class="text-sm text-mnt-muted py-4 text-center">
      No tasks
    </div>

    <!-- Flat list -->
    <div v-else-if="!groupByNode" class="space-y-1">
      <div
        v-for="task in sortedTasks"
        :key="task.task_id"
        class="bg-mnt-surface rounded-lg border border-mnt-default px-3 py-2 flex items-center justify-between"
      >
        <div class="flex items-center gap-2 min-w-0">
          <div :class="['w-2 h-2 rounded-full flex-shrink-0', stateDot(task.state)]" />
          <span class="text-xs text-mnt-secondary font-mono">#{{ task.slot }}</span>
          <span :class="['text-xs font-medium', stateColor(task.state)]">{{ task.state }}</span>
          <span v-if="task.node_hostname" class="text-xs text-mnt-muted truncate">{{ task.node_hostname }}</span>
        </div>
        <div class="flex items-center gap-3 text-xs text-mnt-muted flex-shrink-0 ml-2">
          <span v-if="task.error" class="text-mnt-status-down truncate max-w-48" :title="task.error">{{ task.error }}</span>
          <span v-if="task.exit_code !== null && task.exit_code !== 0" class="text-mnt-status-down">exit {{ task.exit_code }}</span>
          <span class="tabular-nums">{{ timeAgo(task.timestamp) }}</span>
        </div>
      </div>
    </div>

    <!-- Grouped by node -->
    <div v-else-if="tasksByNode" class="space-y-3">
      <div v-for="[nodeId, group] in tasksByNode" :key="nodeId">
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">
          {{ group.hostname }} ({{ group.tasks.length }})
        </p>
        <div class="space-y-1">
          <div
            v-for="task in group.tasks"
            :key="task.task_id"
            class="bg-mnt-surface rounded-lg border border-mnt-default px-3 py-2 flex items-center justify-between"
          >
            <div class="flex items-center gap-2 min-w-0">
              <div :class="['w-2 h-2 rounded-full flex-shrink-0', stateDot(task.state)]" />
              <span class="text-xs text-mnt-secondary font-mono">#{{ task.slot }}</span>
              <span :class="['text-xs font-medium', stateColor(task.state)]">{{ task.state }}</span>
            </div>
            <div class="flex items-center gap-3 text-xs text-mnt-muted flex-shrink-0 ml-2">
              <span v-if="task.error" class="text-mnt-status-down truncate max-w-48" :title="task.error">{{ task.error }}</span>
              <span v-if="task.exit_code !== null && task.exit_code !== 0" class="text-mnt-status-down">exit {{ task.exit_code }}</span>
              <span class="tabular-nums">{{ timeAgo(task.timestamp) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
