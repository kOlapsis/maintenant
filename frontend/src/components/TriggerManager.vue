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
import { ref, onMounted } from 'vue'
import { Plus, Edit2, Trash2 } from 'lucide-vue-next'
import { useTriggersStore } from '@/stores/triggers'
import { useChannelsStore } from '@/stores/channels'
import { useConfirm } from '@/composables/useConfirm'
import TriggerEditor from '@/components/TriggerEditor.vue'
import type { AlertTrigger } from '@/types/triggers'

const store = useTriggersStore()
const channelsStore = useChannelsStore()
const confirm = useConfirm()

const showEditor = ref(false)
const editingTrigger = ref<AlertTrigger | null>(null)

function openCreate() {
  editingTrigger.value = null
  showEditor.value = true
}

function openEdit(t: AlertTrigger) {
  editingTrigger.value = t
  showEditor.value = true
}

function closeEditor() {
  showEditor.value = false
  editingTrigger.value = null
}

async function handleSaved() {
  closeEditor()
  await store.fetchTriggers()
}

async function handleDelete(t: AlertTrigger) {
  const ok = await confirm({
    title: 'Delete trigger',
    message: `Remove "${t.name}"? Channels referenced by this trigger will no longer receive alerts via it.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  await store.remove(t.id)
}

async function handleToggleEnabled(t: AlertTrigger) {
  await store.update(t.id, {
    name: t.name,
    filter_severities: t.filter_severities,
    filter_sources: t.filter_sources,
    filter_scopes: t.filter_scopes,
    filter_tags: t.filter_tags,
    enabled: !t.enabled,
    channel_ids: t.channel_ids,
  })
}

function channelNames(ids: number[]): string {
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

onMounted(async () => {
  await Promise.all([store.fetchTriggers(), channelsStore.fetchChannels()])
})
</script>

<template>
  <div class="space-y-4">
    <!-- Action bar -->
    <div class="flex items-center justify-between">
      <p class="text-xs text-slate-500">
        <template v-if="store.triggers.length > 0">
          {{ store.triggers.length }} {{ store.triggers.length === 1 ? 'trigger' : 'triggers' }}
        </template>
      </p>
      <button
        v-if="!showEditor"
        class="inline-flex items-center gap-2 px-4 py-2 bg-pb-green-600 hover:bg-pb-green-500 text-slate-950 rounded-lg text-xs font-bold transition-all shadow-lg shadow-pb-green-500/20"
        @click="openCreate"
      >
        <Plus :size="13" />
        New trigger
      </button>
    </div>

    <!-- Editor -->
    <TriggerEditor
      v-if="showEditor"
      :trigger="editingTrigger"
      @saved="handleSaved"
      @cancel="closeEditor"
    />

    <!-- Empty state -->
    <div
      v-if="!showEditor && !store.loading && store.triggers.length === 0"
      class="rounded-xl border border-slate-800 bg-[#12151C] p-8 text-center"
    >
      <p class="text-sm text-slate-400">No triggers configured.</p>
      <p class="mt-1 text-xs text-slate-500">
        Without triggers, alerts are not dispatched anywhere by default. Create one above.
      </p>
    </div>

    <!-- List -->
    <div v-if="!showEditor && store.triggers.length > 0" class="space-y-2">
      <div
        v-for="t in store.triggers"
        :key="t.id"
        class="flex items-start justify-between rounded-xl border border-slate-800 bg-[#12151C] p-4 hover:bg-slate-800/25 transition-all group"
      >
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm font-medium text-white">{{ t.name }}</span>
            <span
              v-if="!t.enabled"
              class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-slate-800 text-slate-500"
            >disabled</span>
          </div>
          <p class="mt-1 text-[11px] text-slate-500">{{ summarizeFilter(t) }}</p>
          <p class="mt-1 text-[11px] text-slate-400">
            <span class="text-slate-600">→</span> {{ channelNames(t.channel_ids) }}
          </p>
        </div>
        <div class="flex items-center gap-2 ml-4 shrink-0">
          <button
            class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus:outline-none"
            :class="t.enabled ? 'bg-pb-green-600' : 'bg-slate-700'"
            :title="t.enabled ? 'Click to disable' : 'Click to enable'"
            @click="handleToggleEnabled(t)"
          >
            <span
              class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform"
              :class="t.enabled ? 'translate-x-4' : 'translate-x-0'"
            />
          </button>
          <button
            class="rounded-lg border border-slate-700 px-2.5 py-1 text-xs text-slate-300 hover:bg-slate-800/50 transition-all flex items-center gap-1"
            @click="openEdit(t)"
          >
            <Edit2 :size="11" /> Edit
          </button>
          <button
            class="rounded-lg border border-pb-status-down/40 px-2.5 py-1 text-xs text-pb-status-down hover:bg-pb-status-down/10 transition-all flex items-center gap-1"
            @click="handleDelete(t)"
          >
            <Trash2 :size="11" /> Delete
          </button>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div
      v-if="store.error"
      class="px-4 py-3 rounded-lg bg-pb-status-down/10 border border-pb-status-down/30 text-xs text-pb-status-down"
    >
      {{ store.error }}
    </div>
  </div>
</template>
