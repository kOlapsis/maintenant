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
import { Plus } from 'lucide-vue-next'
import { useTriggersStore } from '@/stores/triggers'
import { useChannelsStore } from '@/stores/channels'
import { useConfirm } from '@/composables/useConfirm'
import TriggerEditor from '@/components/TriggerEditor.vue'
import TriggerList from '@/components/TriggerList.vue'
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

onMounted(async () => {
  await Promise.all([store.fetchTriggers(), channelsStore.fetchChannels()])
})
</script>

<template>
  <div class="space-y-4">
    <!-- Action bar -->
    <div class="flex items-center justify-between">
      <p class="text-xs text-mnt-muted">
        <template v-if="store.triggers.length > 0">
          {{ store.triggers.length }} {{ store.triggers.length === 1 ? 'trigger' : 'triggers' }}
        </template>
      </p>
      <button
        v-if="!showEditor"
        class="inline-flex items-center gap-2 px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted rounded-lg text-xs font-bold transition-all shadow-lg shadow-mnt-green-500/20"
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
      class="rounded-xl border border-mnt-default bg-mnt-surface p-8 text-center"
    >
      <p class="text-sm text-mnt-muted">No triggers configured.</p>
      <p class="mt-1 text-xs text-mnt-muted">
        Without triggers, alerts are not dispatched anywhere by default. Create one above.
      </p>
    </div>

    <!-- List -->
    <TriggerList
      v-if="!showEditor && store.triggers.length > 0"
      :triggers="store.triggers"
      @edit="openEdit"
      @delete="handleDelete"
      @toggle="handleToggleEnabled"
    />

    <!-- Error -->
    <div
      v-if="store.error"
      class="px-4 py-3 rounded-lg bg-mnt-status-down/10 border border-mnt-status-down/30 text-xs text-mnt-status-down"
    >
      {{ store.error }}
    </div>
  </div>
</template>
