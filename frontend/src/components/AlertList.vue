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
import { ref, watch, inject } from 'vue'
import { useRouter } from 'vue-router'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import type { Alert, ListAlertsParams } from '@/services/alertApi'

const router = useRouter()
const detailSlideOver = inject(detailSlideOverKey)!
const store = useAlertsStore()

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
  store.fetchAlerts(buildParams())
}

function loadMore() {
  const last = store.alerts[store.alerts.length - 1]
  if (!last) return
  store.fetchAlerts({ ...buildParams(), before: last.fired_at })
}

watch([sourceFilter, severityFilter, statusFilter], () => applyFilters())

function formatTime(ts: string): string {
  return new Date(ts).toLocaleString()
}

const severityColors: Record<string, { bg: string; color: string }> = {
  critical: { bg: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)' },
  warning: { bg: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)' },
  info: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--pb-accent)' },
}

const statusColors: Record<string, { bg: string; color: string }> = {
  active: { bg: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)' },
  resolved: { bg: 'var(--pb-status-ok-bg)', color: 'var(--pb-status-ok)' },
  silenced: { bg: 'var(--pb-bg-elevated)', color: 'var(--pb-text-muted)' },
}

const ENTITY_TYPES: ReadonlySet<string> = new Set(['container', 'heartbeat', 'certificate'])

function openEntityDetail(alert: Alert) {
  if (alert.entity_type === 'endpoint' && alert.entity_id) {
    router.push({ name: 'endpoints' })
    return
  }
  if (!alert.entity_id || !ENTITY_TYPES.has(alert.entity_type)) return
  detailSlideOver.openDetail(alert.entity_type as EntityType, alert.entity_id)
}

const selectStyle = 'background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)'
</script>

<template>
  <div>
    <!-- Filters -->
    <div class="mb-4 flex flex-wrap gap-3">
      <select
        v-model="sourceFilter"
        class="rounded-md border px-3 py-1.5 text-sm outline-none"
        :style="selectStyle"
      >
        <option value="">All sources</option>
        <option value="container">Container</option>
        <option value="endpoint">Endpoint</option>
        <option value="heartbeat">Heartbeat</option>
        <option value="certificate">Certificate</option>
        <option value="resource">Resource</option>
      </select>

      <select
        v-model="severityFilter"
        class="rounded-md border px-3 py-1.5 text-sm outline-none"
        :style="selectStyle"
      >
        <option value="">All severities</option>
        <option value="critical">Critical</option>
        <option value="warning">Warning</option>
        <option value="info">Info</option>
      </select>

      <select
        v-model="statusFilter"
        class="rounded-md border px-3 py-1.5 text-sm outline-none"
        :style="selectStyle"
      >
        <option value="">All statuses</option>
        <option value="active">Active</option>
        <option value="resolved">Resolved</option>
        <option value="silenced">Silenced</option>
      </select>
    </div>

    <!-- Mobile card list -->
    <div class="md:hidden space-y-2">
      <div v-if="store.alerts.length === 0 && !store.loading" class="rounded-lg border p-8 text-center text-sm" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default); color: var(--pb-text-muted)">No alerts found</div>
      <div
        v-for="alert in store.alerts"
        :key="'m-' + alert.id"
        class="rounded-lg border p-3 cursor-pointer transition-colors"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
        @click="openEntityDetail(alert)"
      >
        <div class="flex items-center justify-between gap-2 mb-1.5">
          <div class="flex items-center gap-2">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                background: (severityColors[alert.severity] || { bg: 'var(--pb-bg-elevated)' }).bg,
                color: (severityColors[alert.severity] || { color: 'var(--pb-text-secondary)' }).color,
              }"
            >{{ alert.severity }}</span>
            <span class="rounded px-1.5 py-0.5 text-xs font-medium" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">{{ alert.source }}</span>
          </div>
          <span
            class="rounded-full px-2 py-0.5 text-xs font-medium"
            :style="{
              background: (statusColors[alert.status] || { bg: 'var(--pb-bg-elevated)' }).bg,
              color: (statusColors[alert.status] || { color: 'var(--pb-text-secondary)' }).color,
            }"
          >{{ alert.status }}</span>
        </div>
        <p class="text-sm truncate" style="color: var(--pb-text-primary)">{{ alert.message }}</p>
        <div class="flex items-center justify-between mt-1.5 text-xs" style="color: var(--pb-text-muted)">
          <span>{{ alert.entity_name || '-' }}</span>
          <span>{{ formatTime(alert.fired_at) }}</span>
        </div>
      </div>
    </div>

    <!-- Desktop table -->
    <div
      class="hidden md:block overflow-x-auto rounded-lg border"
      style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
    >
      <table class="min-w-full">
        <thead>
          <tr style="background: var(--pb-bg-elevated); border-bottom: 1px solid var(--pb-border-default)">
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Severity</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Source</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Message</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Entity</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Time</th>
            <th class="px-4 py-2 text-left text-xs font-medium uppercase" style="color: var(--pb-text-muted)">Status</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="store.alerts.length === 0 && !store.loading">
            <td colspan="6" class="px-4 py-8 text-center text-sm" style="color: var(--pb-text-muted)">No alerts found</td>
          </tr>
          <tr
            v-for="alert in store.alerts"
            :key="alert.id"
            class="transition-colors cursor-pointer"
            :style="{ borderBottom: '1px solid var(--pb-border-subtle)' }"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--pb-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="openEntityDetail(alert)"
          >
            <td class="px-4 py-2">
              <span
                class="rounded-full px-2 py-0.5 text-xs font-medium"
                :style="{
                  background: (severityColors[alert.severity] || { bg: 'var(--pb-bg-elevated)' }).bg,
                  color: (severityColors[alert.severity] || { color: 'var(--pb-text-secondary)' }).color,
                }"
              >
                {{ alert.severity }}
              </span>
            </td>
            <td class="px-4 py-2">
              <span
                class="rounded px-1.5 py-0.5 text-xs font-medium"
                style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
              >
                {{ alert.source }}
              </span>
            </td>
            <td class="max-w-md truncate px-4 py-2 text-sm" style="color: var(--pb-text-primary)">{{ alert.message }}</td>
            <td class="px-4 py-2 text-sm" style="color: var(--pb-text-muted)">{{ alert.entity_name || '-' }}</td>
            <td class="whitespace-nowrap px-4 py-2 text-xs" style="color: var(--pb-text-muted)">{{ formatTime(alert.fired_at) }}</td>
            <td class="px-4 py-2">
              <span
                class="rounded-full px-2 py-0.5 text-xs font-medium"
                :style="{
                  background: (statusColors[alert.status] || { bg: 'var(--pb-bg-elevated)' }).bg,
                  color: (statusColors[alert.status] || { color: 'var(--pb-text-secondary)' }).color,
                }"
              >
                {{ alert.status }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Load more -->
    <div v-if="store.hasMore" class="mt-4 text-center">
      <button
        @click="loadMore"
        :disabled="store.loading"
        class="rounded-md border px-4 py-2 text-sm disabled:opacity-50 transition-colors"
        style="border-color: var(--pb-border-default); color: var(--pb-text-secondary); background: var(--pb-bg-surface)"
      >
        {{ store.loading ? 'Loading...' : 'Load more' }}
      </button>
    </div>
  </div>
</template>
