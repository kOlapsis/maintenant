<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref, watch, inject, nextTick, onUnmounted } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { getAlert, type Alert, type ListAlertsParams } from '@/services/alertApi'
import { ApiError } from '@/services/apiFetch'
import { humanizeAlertType } from '@/utils/alertLabels'
import AcknowledgeButton from '@/components/ui/AcknowledgeButton.vue'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const props = defineProps<{ linkedId?: string }>()

const detailSlideOver = inject(detailSlideOverKey)!
const store = useAlertsStore()

const linkedError = ref<string | null>(null)
const root = ref<HTMLElement | null>(null)
let userScrolled = false

watch(
  () => props.linkedId,
  async (id) => {
    store.unpinAlert()
    linkedError.value = null
    userScrolled = false
    if (!id) return
    try {
      const alert = await getAlert(id)
      if (props.linkedId === id) store.pinAlert(alert)
    } catch (e) {
      if (props.linkedId !== id) return
      linkedError.value = e instanceof ApiError && e.status === 404
        ? 'This alert no longer exists. Alerts that are no longer active are purged after 90 days.'
        : 'This alert could not be loaded.'
    }
  },
  { immediate: true },
)

const USER_SCROLL_EVENTS = ['wheel', 'touchmove', 'keydown'] as const

function onUserScroll() {
  userScrolled = true
}

USER_SCROLL_EVENTS.forEach((e) => window.addEventListener(e, onUserScroll, { passive: true }))

watch(
  () => [props.linkedId, store.alerts.length, store.loading, store.totalActiveCount] as const,
  async ([id]) => {
    if (!id || userScrolled || store.loading || !store.alerts.some((a) => a.id === id)) return
    await nextTick()
    const target = Array.from(root.value?.querySelectorAll<HTMLElement>('[data-alert-id]') ?? [])
      .find((el) => el.dataset.alertId === id && el.offsetParent !== null)
    target?.scrollIntoView({ block: 'center', inline: 'start' })
  },
  { immediate: true },
)

onUnmounted(() => {
  USER_SCROLL_EVENTS.forEach((e) => window.removeEventListener(e, onUserScroll))
  store.unpinAlert()
})

const sourceFilter = ref('')
const severityFilter = ref('')
const statusFilter = ref('')

function buildParams(): ListAlertsParams {
  const params: ListAlertsParams = { limit: 50 }
  if (sourceFilter.value) params.source = sourceFilter.value
  if (severityFilter.value) params.severity = severityFilter.value
  if (statusFilter.value) params.status = statusFilter.value
  return params
}

function applyFilters() {
  store.unpinAlert()
  store.fetchAlerts(buildParams())
}

function loadMore() {
  const last = store.alerts[store.alerts.length - 1]
  if (!last) return
  store.fetchAlerts({ ...buildParams(), before: last.fired_at, before_id: last.id })
}

watch([sourceFilter, severityFilter, statusFilter], () => applyFilters())

function formatTime(ts: string): string {
  return new Date(ts).toLocaleString()
}

const severityColors: Record<string, { bg: string; color: string }> = {
  critical: { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' },
  warning: { bg: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)' },
  info: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--mnt-accent)' },
}

const statusColors: Record<string, { bg: string; color: string }> = {
  active: { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' },
  resolved: { bg: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok)' },
  silenced: { bg: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-muted)' },
}

const ENTITY_TYPES: ReadonlySet<string> = new Set(['container', 'heartbeat', 'certificate'])

function alertTitle(alert: Alert): string {
  const humanized = humanizeAlertType(alert.source, alert.alert_type)
  return humanized === alert.alert_type ? alert.message : humanized
}

function openEntityDetail(alert: Alert) {
  // Update alerts carry entity_type='container' but should open the update panel.
  if (alert.source === 'update' && alert.entity_id) {
    detailSlideOver.openDetail('update', alert.entity_id)
    return
  }
  if (alert.entity_type === 'endpoint' && alert.entity_id) {
    detailSlideOver.openDetail('endpoint', alert.entity_id)
    return
  }
  if (!alert.entity_id || !ENTITY_TYPES.has(alert.entity_type)) return
  detailSlideOver.openDetail(alert.entity_type as EntityType, alert.entity_id)
}

const sourceOptions = [
  { value: '', label: 'All sources' },
  { value: 'container', label: 'Container' },
  { value: 'endpoint', label: 'Endpoint' },
  { value: 'heartbeat', label: 'Heartbeat' },
  { value: 'certificate', label: 'Certificate' },
  { value: 'resource', label: 'Resource' },
  { value: 'agent', label: 'Agent' },
]

const severityOptions = [
  { value: '', label: 'All severities' },
  { value: 'critical', label: 'Critical' },
  { value: 'warning', label: 'Warning' },
  { value: 'info', label: 'Info' },
]

const statusOptions = [
  { value: '', label: 'All statuses' },
  { value: 'active', label: 'Active' },
  { value: 'resolved', label: 'Resolved' },
  { value: 'silenced', label: 'Silenced' },
]
</script>

<template>
  <div ref="root">
    <InlineAlert v-if="linkedError" severity="info" title="Linked alert not found" class="mb-4">
      {{ linkedError }}
    </InlineAlert>

    <!-- Filters -->
    <div class="mb-4 flex flex-wrap gap-3">
      <SelectInput v-model="sourceFilter" size="sm" class="w-auto" :options="sourceOptions" />
      <SelectInput v-model="severityFilter" size="sm" class="w-auto" :options="severityOptions" />
      <SelectInput v-model="statusFilter" size="sm" class="w-auto" :options="statusOptions" />
    </div>

    <!-- Mobile card list -->
    <div class="md:hidden space-y-2">
      <div v-if="store.alerts.length === 0 && !store.loading" class="rounded-lg border p-8 text-center text-sm" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default); color: var(--mnt-text-muted)">No alerts found</div>
      <div
        v-for="alert in store.alerts"
        :key="'m-' + alert.id"
        :data-alert-id="alert.id"
        class="alert-card rounded-lg border p-3 cursor-pointer transition-colors"
        :class="{ 'alert-linked': alert.id === linkedId }"
        :aria-current="alert.id === linkedId ? 'true' : undefined"
        @click="openEntityDetail(alert)"
      >
        <div class="flex items-center justify-between gap-2 mb-1.5">
          <div class="flex items-center gap-2">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                background: (severityColors[alert.severity] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                color: (severityColors[alert.severity] || { color: 'var(--mnt-text-secondary)' }).color,
              }"
            >{{ alert.severity }}</span>
            <span class="rounded px-1.5 py-0.5 text-xs font-medium" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">{{ alert.source }}</span>
          </div>
          <div class="flex items-center gap-2">
            <AcknowledgeButton :alert="alert" />
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                background: (statusColors[alert.status] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                color: (statusColors[alert.status] || { color: 'var(--mnt-text-secondary)' }).color,
              }"
            >{{ alert.status }}</span>
          </div>
        </div>
        <p class="text-sm truncate" style="color: var(--mnt-text-primary)">{{ alertTitle(alert) }}</p>
        <div class="flex items-center justify-between mt-1.5 text-xs" style="color: var(--mnt-text-muted)">
          <span>{{ alert.entity_name || '-' }}</span>
          <span>{{ formatTime(alert.fired_at) }}</span>
        </div>
      </div>
    </div>

    <!-- Desktop table -->
    <div
      class="hidden md:block overflow-x-auto rounded-lg border"
      style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
    >
      <table class="min-w-full">
        <thead>
          <tr style="background: var(--mnt-bg-elevated); border-bottom: 1px solid var(--mnt-border-default)">
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Severity</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Source</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Message</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Entity</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Time</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--mnt-text-muted)">Status</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="store.alerts.length === 0 && !store.loading">
            <td colspan="6" class="px-4 py-8 text-center text-sm" style="color: var(--mnt-text-muted)">No alerts found</td>
          </tr>
          <tr
            v-for="alert in store.alerts"
            :key="alert.id"
            :data-alert-id="alert.id"
            class="alert-row transition-colors cursor-pointer"
            :class="{ 'alert-linked': alert.id === linkedId }"
            :aria-current="alert.id === linkedId ? 'true' : undefined"
            @click="openEntityDetail(alert)"
          >
            <td class="px-4 py-2">
              <span
                class="rounded-full px-2 py-0.5 text-xs font-medium"
                :style="{
                  background: (severityColors[alert.severity] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                  color: (severityColors[alert.severity] || { color: 'var(--mnt-text-secondary)' }).color,
                }"
              >
                {{ alert.severity }}
              </span>
            </td>
            <td class="px-4 py-2">
              <span
                class="rounded px-1.5 py-0.5 text-xs font-medium"
                style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
              >
                {{ alert.source }}
              </span>
            </td>
            <td class="max-w-md truncate px-4 py-2 text-sm" style="color: var(--mnt-text-primary)">{{ alertTitle(alert) }}</td>
            <td class="px-4 py-2 text-sm" style="color: var(--mnt-text-muted)">{{ alert.entity_name || '-' }}</td>
            <td class="whitespace-nowrap px-4 py-2 text-xs" style="color: var(--mnt-text-muted)">{{ formatTime(alert.fired_at) }}</td>
            <td class="px-4 py-2">
              <div class="flex items-center gap-2">
                <span
                  class="rounded-full px-2 py-0.5 text-xs font-medium"
                  :style="{
                    background: (statusColors[alert.status] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                    color: (statusColors[alert.status] || { color: 'var(--mnt-text-secondary)' }).color,
                  }"
                >
                  {{ alert.status }}
                </span>
                <AcknowledgeButton :alert="alert" />
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Load more -->
    <div v-if="store.hasMore" class="mt-4 text-center">
      <UiButton variant="secondary" size="sm" :loading="store.loading" @click="loadMore">
        {{ store.loading ? 'Loading...' : 'Load more' }}
      </UiButton>
    </div>
  </div>
</template>

<style scoped>
.alert-card {
  background: var(--mnt-bg-surface);
  border-color: var(--mnt-border-default);
}
.alert-row {
  border-bottom: 1px solid var(--mnt-border-subtle);
}
.alert-row:hover {
  background: var(--mnt-bg-hover);
}
.alert-linked,
.alert-linked:hover {
  background: color-mix(in srgb, var(--mnt-accent) 12%, var(--mnt-bg-surface));
  box-shadow: inset 3px 0 0 var(--mnt-accent);
}
.alert-card.alert-linked {
  border-color: var(--mnt-accent);
}
</style>
