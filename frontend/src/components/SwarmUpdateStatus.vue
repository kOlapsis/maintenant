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
import { timeAgo } from '@/utils/time'

export interface UpdateStatusData {
  state: string
  message?: string
  started_at?: string
  completed_at?: string
  old_image?: string
  new_image?: string
  tasks_updated?: number
  tasks_total?: number
}

const props = defineProps<{
  status: UpdateStatusData
}>()

const progressPercent = computed(() => {
  if (!props.status.tasks_total || props.status.tasks_total === 0) return 0
  return Math.round((props.status.tasks_updated ?? 0) / props.status.tasks_total * 100)
})

function stateStyle(state: string): string {
  switch (state) {
    case 'updating': return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
    case 'completed': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
    case 'paused': return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
    case 'rollback_started': case 'rollback_paused': return 'text-red-400 bg-red-400/10 border-red-400/20'
    case 'rollback_completed': return 'text-red-400 bg-red-400/10 border-red-400/20'
    default: return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
  }
}

function stateLabel(state: string): string {
  return state.replace(/_/g, ' ')
}

function shortImage(image: string | undefined): string {
  if (!image) return ''
  const atIdx = image.indexOf('@sha256:')
  if (atIdx > 0) return image.substring(0, atIdx)
  return image
}
</script>

<template>
  <div class="bg-pb-surface rounded-xl border border-slate-800 p-4">
    <div class="flex items-center justify-between mb-3">
      <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Rolling Update</p>
      <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', stateStyle(status.state)]">
        {{ stateLabel(status.state) }}
      </span>
    </div>

    <!-- Progress bar -->
    <div v-if="status.tasks_total && status.tasks_total > 0" class="mb-3">
      <div class="flex items-center justify-between text-xs text-slate-400 mb-1">
        <span>{{ status.tasks_updated ?? 0 }}/{{ status.tasks_total }} tasks updated</span>
        <span class="tabular-nums">{{ progressPercent }}%</span>
      </div>
      <div class="h-1.5 bg-pb-primary border border-slate-800 rounded-full overflow-hidden">
        <div
          class="h-full rounded-full transition-all duration-500"
          :class="status.state === 'updating' ? 'bg-sky-500' : status.state === 'completed' ? 'bg-emerald-500' : 'bg-amber-500'"
          :style="{ width: `${progressPercent}%` }"
        />
      </div>
    </div>

    <!-- Image transition -->
    <div v-if="status.old_image || status.new_image" class="flex items-center gap-2 text-xs mb-2">
      <span v-if="status.old_image" class="text-slate-500 truncate max-w-40">{{ shortImage(status.old_image) }}</span>
      <span v-if="status.old_image && status.new_image" class="text-slate-600">→</span>
      <span v-if="status.new_image" class="text-pb-secondary truncate max-w-40">{{ shortImage(status.new_image) }}</span>
    </div>

    <!-- Timestamps -->
    <div class="flex items-center gap-4 text-xs text-slate-500">
      <span v-if="status.started_at">Started {{ timeAgo(status.started_at) }}</span>
      <span v-if="status.completed_at">Completed {{ timeAgo(status.completed_at) }}</span>
    </div>

    <!-- Message -->
    <p v-if="status.message" class="text-xs text-slate-500 mt-1">{{ status.message }}</p>
  </div>
</template>
