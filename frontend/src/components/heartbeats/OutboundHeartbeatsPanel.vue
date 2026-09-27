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
import { onMounted, ref, watch } from 'vue'
import { Send, Pencil, Trash2, ArrowUpRight } from 'lucide-vue-next'
import { useOutboundHeartbeatsStore } from '@/stores/outboundHeartbeats'
import { useConfirm } from '@/composables/useConfirm'
import { formatInterval } from '@/utils/heartbeatFormat'
import { timeAgo } from '@/utils/time'
import type { OutboundHeartbeat } from '@/services/outboundHeartbeatApi'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import QuotaRefusal from '@/components/QuotaRefusal.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import type { ChipTone } from '@/components/ui/listFilters'

const showCreateForm = defineModel<boolean>('showCreateForm', { default: false })

const store = useOutboundHeartbeatsStore()
const confirm = useConfirm()

const editingId = ref<string | null>(null)
const formError = ref<unknown>(null)
const sendingId = ref<string | null>(null)
const togglingId = ref<string | null>(null)

const intervalPresets = [
  { label: '30s', value: 30 },
  { label: '1m', value: 60 },
  { label: '5m', value: 300 },
  { label: '15m', value: 900 },
  { label: '1h', value: 3600 },
  { label: '6h', value: 21600 },
  { label: '12h', value: 43200 },
  { label: '24h', value: 86400 },
]

function blankForm() {
  return { name: '', url: '', interval_seconds: 300, enabled: true }
}

const form = ref(blankForm())

watch(showCreateForm, (open) => {
  if (open && !editingId.value) {
    form.value = blankForm()
    formError.value = null
  }
})

function startEdit(hb: OutboundHeartbeat) {
  editingId.value = hb.id
  form.value = {
    name: hb.name,
    url: hb.url,
    interval_seconds: hb.interval_seconds,
    enabled: hb.enabled,
  }
  formError.value = null
  showCreateForm.value = true
}

function closeForm() {
  showCreateForm.value = false
  editingId.value = null
}

async function handleSubmit() {
  formError.value = null
  try {
    if (editingId.value) {
      await store.update(editingId.value, form.value)
    } else {
      await store.create(form.value)
    }
    closeForm()
  } catch (e) {
    formError.value = e
  }
}

async function handleDelete(hb: OutboundHeartbeat) {
  const ok = await confirm({
    title: 'Delete target',
    message: `Remove "${hb.name}"? Maintenant will stop pinging it.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  await store.remove(hb.id)
}

async function handleSendNow(hb: OutboundHeartbeat) {
  sendingId.value = hb.id
  try {
    await store.sendNow(hb.id)
  } finally {
    sendingId.value = null
  }
}

async function toggleEnabled(hb: OutboundHeartbeat) {
  togglingId.value = hb.id
  try {
    await store.update(hb.id, {
      name: hb.name,
      url: hb.url,
      interval_seconds: hb.interval_seconds,
      enabled: !hb.enabled,
    })
  } finally {
    togglingId.value = null
  }
}

function tone(hb: OutboundHeartbeat): ChipTone {
  if (!hb.enabled) return 'neutral'
  if (hb.last_error) return 'down'
  if (hb.last_status_code != null) return hb.last_status_code >= 200 && hb.last_status_code < 300 ? 'ok' : 'down'
  return 'unknown'
}

function lastSendLabel(hb: OutboundHeartbeat): string {
  if (!hb.last_sent_at) return 'Never'
  const status = hb.last_error || (hb.last_status_code != null ? `HTTP ${hb.last_status_code}` : '')
  return status ? `${timeAgo(hb.last_sent_at)} · ${status}` : timeAgo(hb.last_sent_at)
}

function lastSendColor(hb: OutboundHeartbeat): string {
  if (hb.last_error) return 'var(--mnt-status-down-text)'
  if (hb.last_status_code != null) {
    return hb.last_status_code >= 200 && hb.last_status_code < 300
      ? 'var(--mnt-status-ok-text)'
      : 'var(--mnt-status-down-text)'
  }
  return 'var(--mnt-text-muted)'
}

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true, width: 'minmax(0, 1.4fr)' },
  { key: 'url', label: 'URL', width: 'minmax(0, 2fr)', priority: 'sm' },
  { key: 'interval', label: 'Interval', sortable: true, align: 'right', width: '90px', priority: 'md' },
  { key: 'enabled', label: 'Enabled', align: 'right', width: '96px' },
  { key: 'last_send', label: 'Last send', sortable: true, align: 'right', width: '190px', priority: 'lg' },
  { key: 'actions', label: 'Actions', align: 'right', width: '116px' },
]

function sortValue(hb: OutboundHeartbeat, key: string): string | number | undefined {
  switch (key) {
    case 'name':
      return hb.name
    case 'interval':
      return hb.interval_seconds
    case 'last_send':
      return hb.last_sent_at ? new Date(hb.last_sent_at).getTime() : undefined
    default:
      return undefined
  }
}

onMounted(() => {
  store.fetchOutboundHeartbeats()
})
</script>

<template>
  <div>
    <div
      v-if="showCreateForm"
      class="mb-6 p-4"
      :style="{
        backgroundColor: 'var(--mnt-bg-surface)',
        border: '1px solid var(--mnt-border-default)',
        borderRadius: 'var(--mnt-radius-lg)',
      }"
    >
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--mnt-text-primary)' }">
        {{ editingId ? 'Edit outgoing target' : 'Add outgoing target' }}
      </h3>
      <QuotaRefusal v-if="formError" :error="formError" />
      <form class="flex flex-col gap-3" @submit.prevent="handleSubmit">
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Name</label>
          <input
            v-model="form.name"
            type="text"
            placeholder="e.g., Watched by DR site"
            :style="{
              width: '100%',
              borderRadius: 'var(--mnt-radius-md)',
              border: '1px solid var(--mnt-border-default)',
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Ping URL</label>
          <input
            v-model="form.url"
            type="url"
            placeholder="https://other-instance.example.com/ping/…"
            class="font-mono"
            :style="{
              width: '100%',
              borderRadius: 'var(--mnt-radius-md)',
              border: '1px solid var(--mnt-border-default)',
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.8125rem',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Ping every</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.interval_seconds === preset.value
                  ? '1px solid var(--mnt-accent)'
                  : '1px solid var(--mnt-border-default)',
                backgroundColor: form.interval_seconds === preset.value
                  ? 'var(--mnt-accent)'
                  : 'transparent',
                color: form.interval_seconds === preset.value
                  ? 'var(--mnt-text-inverted)'
                  : 'var(--mnt-text-secondary)',
              }"
              @click="form.interval_seconds = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>
        <label class="flex min-h-[44px] items-center gap-2">
          <input v-model="form.enabled" type="checkbox" class="focus-ring h-4 w-4" />
          <span class="text-xs font-semibold text-mnt-secondary">Enabled</span>
        </label>
        <div class="flex items-center gap-2">
          <button
            type="submit"
            class="min-h-[44px]"
            :style="{
              borderRadius: 'var(--mnt-radius-lg)',
              backgroundColor: 'var(--mnt-accent)',
              color: 'var(--mnt-text-inverted)',
              padding: '0.5rem 1rem',
              fontSize: '0.875rem',
              fontWeight: '500',
            }"
          >
            {{ editingId ? 'Save' : 'Create' }}
          </button>
          <button
            type="button"
            class="focus-ring min-h-[44px] rounded-lg border border-mnt-default px-4 text-sm font-medium text-mnt-secondary hover:text-mnt-primary"
            @click="closeForm"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>

    <LoadingSkeleton v-if="store.loading" variant="cards" :count="4" />

    <ErrorState v-else-if="store.error" :message="store.error" />

    <EmptyState
      v-else-if="store.outboundHeartbeats.length === 0"
      :icon="ArrowUpRight"
      title="No outgoing targets"
      description="Add the ping URL of a heartbeat monitor created on another Maintenant instance. This instance will ping it on schedule, so the other one alerts if this one goes quiet."
    >
      <template #action>
        <button
          class="min-h-[44px] rounded-lg px-4 text-sm font-medium"
          style="background-color: var(--mnt-accent); color: var(--mnt-text-inverted); border-radius: var(--mnt-radius-lg)"
          @click="showCreateForm = true"
        >
          Add your first target
        </button>
      </template>
    </EmptyState>

    <DataTable
      v-else
      :columns="columns"
      :rows="store.outboundHeartbeats"
      :row-key="(hb: OutboundHeartbeat) => hb.id"
      :sort-value="sortValue"
      :tone="tone"
      default-sort="name"
      caption="Outgoing heartbeat targets"
    >
      <template #cell-name="{ row }">
        <span class="font-medium text-mnt-primary">{{ row.name }}</span>
      </template>
      <template #cell-url="{ row }">
        <span class="font-mono text-xs" :title="row.url">{{ row.url }}</span>
      </template>
      <template #cell-interval="{ row }">
        <span class="font-mono tabular-nums">{{ formatInterval(row.interval_seconds) }}</span>
      </template>
      <template #cell-enabled="{ row }">
        <button
          type="button"
          role="switch"
          :aria-checked="row.enabled"
          :disabled="togglingId === row.id"
          :title="row.enabled ? 'Click to disable' : 'Click to enable'"
          class="focus-ring inline-flex min-h-[32px] items-center rounded-full border px-2.5 py-1 text-xs font-semibold transition-colors disabled:opacity-50"
          :style="row.enabled
            ? { borderColor: 'var(--mnt-status-ok)', backgroundColor: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok-text)' }
            : { borderColor: 'var(--mnt-border-default)', backgroundColor: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-muted)' }"
          @click.stop="toggleEnabled(row)"
        >
          {{ row.enabled ? 'On' : 'Off' }}
        </button>
      </template>
      <template #cell-last_send="{ row }">
        <span class="truncate" :style="{ color: lastSendColor(row) }" :title="row.last_error ?? undefined">
          {{ lastSendLabel(row) }}
        </span>
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <button
            type="button"
            class="focus-ring rounded p-1 text-mnt-muted transition-all hover:bg-mnt-elevated hover:text-mnt-secondary disabled:opacity-50"
            title="Send now"
            :disabled="sendingId === row.id"
            @click.stop="handleSendNow(row)"
          >
            <Send :size="14" />
          </button>
          <button
            type="button"
            class="focus-ring rounded p-1 text-mnt-muted transition-all hover:bg-mnt-elevated hover:text-mnt-secondary"
            title="Edit"
            @click.stop="startEdit(row)"
          >
            <Pencil :size="14" />
          </button>
          <button
            type="button"
            class="focus-ring rounded p-1 text-mnt-muted transition-all hover:bg-mnt-status-down/10 hover:text-mnt-status-down"
            title="Delete"
            @click.stop="handleDelete(row)"
          >
            <Trash2 :size="14" />
          </button>
        </div>
      </template>
    </DataTable>
  </div>
</template>
