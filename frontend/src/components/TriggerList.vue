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
import { Edit2, Trash2 } from 'lucide-vue-next'
import { useChannelsStore } from '@/stores/channels'
import type { AlertTrigger } from '@/types/triggers'

defineProps<{
  triggers: AlertTrigger[]
}>()

const emit = defineEmits<{
  edit: [trigger: AlertTrigger]
  delete: [trigger: AlertTrigger]
  toggle: [trigger: AlertTrigger]
}>()

const channelsStore = useChannelsStore()

function channelNames(ids: string[]): string {
  if (ids.length === 0) return '—'
  const names = ids
    .map((id) => channelsStore.channels.find((c) => c.id === id)?.name)
    .filter((n): n is string => Boolean(n))
  if (names.length === 0) return `${ids.length} unknown`
  return names.join(', ')
}

function summarizeFilter(t: AlertTrigger): string {
  const parts: string[] = []
  if (t.filter_severities) parts.push(`severity: ${t.filter_severities}`)
  if (t.filter_sources) parts.push(`source: ${t.filter_sources}`)
  if (t.filter_scopes) parts.push(`scope: ${t.filter_scopes}`)
  if (t.filter_tags) parts.push(`tags: ${t.filter_tags}`)
  if (parts.length === 0) return 'matches all alerts'
  return parts.join('  ·  ')
}
</script>

<template>
  <div class="space-y-2">
    <div
      v-for="t in triggers"
      :key="t.id"
      class="flex items-start justify-between rounded-xl border border-pb-default bg-pb-surface p-4 hover:bg-pb-elevated transition-all group"
    >
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2 flex-wrap">
          <span class="text-sm font-medium text-pb-primary">{{ t.name }}</span>
          <span
            v-if="!t.enabled"
            class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-pb-elevated text-pb-muted"
          >disabled</span>
        </div>
        <p class="mt-1 text-[11px] text-pb-muted">{{ summarizeFilter(t) }}</p>
        <p class="mt-1 text-[11px] text-pb-muted">
          <span class="text-pb-muted">→</span> {{ channelNames(t.channel_ids) }}
        </p>
      </div>
      <div class="flex items-center gap-2 ml-4 shrink-0">
        <button
          class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none"
          :class="t.enabled ? 'bg-pb-green-600' : 'bg-pb-elevated'"
          :title="t.enabled ? 'Click to disable' : 'Click to enable'"
          @click="emit('toggle', t)"
        >
          <span
            class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform"
            :class="t.enabled ? 'translate-x-4' : 'translate-x-0'"
          />
        </button>
        <button
          class="rounded-lg border border-pb-default px-2.5 py-1 text-xs text-pb-secondary hover:bg-pb-elevated transition-all flex items-center gap-1"
          @click="emit('edit', t)"
        >
          <Edit2 :size="11" /> Edit
        </button>
        <button
          class="rounded-lg border border-pb-status-down/40 px-2.5 py-1 text-xs text-pb-status-down hover:bg-pb-status-down/10 transition-all flex items-center gap-1"
          @click="emit('delete', t)"
        >
          <Trash2 :size="11" /> Delete
        </button>
      </div>
    </div>
  </div>
</template>
