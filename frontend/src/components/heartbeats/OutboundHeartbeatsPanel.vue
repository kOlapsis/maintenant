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
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

const showCreateForm = defineModel<boolean>('showCreateForm', { default: false })

const store = useOutboundHeartbeatsStore()
const confirm = useConfirm()

const editingId = ref<string | null>(null)
const formError = ref<unknown>(null)
const sendingId = ref<string | null>(null)
const togglingId = ref<string | null>(null)

const intervalPresets = [
  { value: '30', label: '30s' },
  { value: '60', label: '1m' },
  { value: '300', label: '5m' },
  { value: '900', label: '15m' },
  { value: '3600', label: '1h' },
  { value: '21600', label: '6h' },
  { value: '43200', label: '12h' },
  { value: '86400', label: '24h' },
]

function blankForm() {
  return { name: '', url: '', interval_seconds: 300, enabled: true }
}

const form = ref(blankForm())

const intervalPreset = ref<string>(String(form.value.interval_seconds))

watch(intervalPreset, (preset) => {
  const seconds = Number(preset)
  if (!Number.isNaN(seconds)) form.value.interval_seconds = seconds
})

watch(showCreateForm, (open) => {
  if (open && !editingId.value) {
    form.value = blankForm()
    intervalPreset.value = String(form.value.interval_seconds)
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
  intervalPreset.value = String(hb.interval_seconds)
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
        <FormField label="Name" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.name"
              type="text"
              placeholder="e.g., Watched by DR site"
              :aria-describedby="describedBy"
              :invalid="invalid"
              required
            />
          </template>
        </FormField>
        <FormField label="Ping URL" hint="HTTPS only, on a public address." required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.url"
              type="url"
              mono
              placeholder="https://other-instance.example.com/ping/…"
              pattern="https://.+"
              :aria-describedby="describedBy"
              :invalid="invalid"
              required
            />
          </template>
        </FormField>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Ping every</label>
          <SegmentedToggle v-model="intervalPreset" :options="intervalPresets" ariaLabel="Ping interval" />
        </div>
        <CheckboxInput v-model="form.enabled" label="Enabled" />
        <div class="flex items-center gap-2">
          <UiButton type="submit" variant="primary">
            {{ editingId ? 'Save' : 'Create' }}
          </UiButton>
          <UiButton type="button" variant="secondary" @click="closeForm">
            Cancel
          </UiButton>
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
        <UiButton variant="primary" @click="showCreateForm = true">
          Add your first target
        </UiButton>
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
        <ToggleSwitch
          :model-value="row.enabled"
          label="Enabled"
          size="sm"
          :disabled="togglingId === row.id"
          @update:model-value="toggleEnabled(row)"
        />
      </template>
      <template #cell-last_send="{ row }">
        <span class="truncate" :style="{ color: lastSendColor(row) }" :title="row.last_error ?? undefined">
          {{ lastSendLabel(row) }}
        </span>
      </template>
      <template #cell-actions="{ row }">
        <div class="flex items-center justify-end gap-1">
          <UiButton
            variant="ghost"
            size="sm"
            :icon="Send"
            aria-label="Send now"
            title="Send now"
            :disabled="sendingId === row.id"
            @click.stop="handleSendNow(row)"
          />
          <UiButton
            variant="ghost"
            size="sm"
            :icon="Pencil"
            aria-label="Edit"
            title="Edit"
            @click.stop="startEdit(row)"
          />
          <UiButton
            variant="ghost"
            size="sm"
            :icon="Trash2"
            aria-label="Delete"
            title="Delete"
            @click.stop="handleDelete(row)"
          />
        </div>
      </template>
    </DataTable>
  </div>
</template>
