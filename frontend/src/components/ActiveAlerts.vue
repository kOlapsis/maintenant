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
import { computed, inject } from 'vue'
import { useRouter } from 'vue-router'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { timeAgo } from '@/utils/time'
import type { Alert } from '@/services/alertApi'

const router = useRouter()
const detailSlideOver = inject(detailSlideOverKey)!
const store = useAlertsStore()

const ENTITY_TYPES: ReadonlySet<string> = new Set(['container', 'endpoint', 'heartbeat', 'certificate'])

function openEntityDetail(alert: Alert) {
  if (alert.source === 'update') {
    router.push({ name: 'updates', query: { container: alert.entity_name } })
    return
  }
  if (!alert.entity_id || !ENTITY_TYPES.has(alert.entity_type)) return
  detailSlideOver.openDetail(alert.entity_type as EntityType, alert.entity_id)
}

const severityConfig: Record<string, { label: string; color: string; bg: string; dot: string }> = {
  critical: {
    label: 'Critical',
    color: 'var(--pb-status-down)',
    bg: 'var(--pb-status-down-bg)',
    dot: 'var(--pb-status-down)',
  },
  warning: {
    label: 'Warning',
    color: 'var(--pb-status-warn)',
    bg: 'var(--pb-status-warn-bg)',
    dot: 'var(--pb-status-warn)',
  },
  info: {
    label: 'Info',
    color: 'var(--pb-accent)',
    bg: 'rgba(59, 130, 246, 0.15)',
    dot: 'var(--pb-accent)',
  },
}

const sections = computed(() =>
  (['critical', 'warning', 'info'] as const)
    .map((key) => ({
      key,
      config: severityConfig[key]!,
      alerts: store.activeAlerts[key] || [],
    }))
    .filter((s) => s.alerts.length > 0),
)
</script>

<template>
  <div>
    <div
      v-if="store.totalActiveCount === 0"
      class="rounded-lg border p-6 text-center"
      style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
    >
      <p class="text-sm" style="color: var(--pb-text-muted)">No active alerts</p>
    </div>

    <div v-else class="space-y-3">
      <div v-for="section in sections" :key="section.key">
        <div class="mb-1.5 flex items-center gap-2">
          <span class="h-2 w-2 rounded-full" :style="{ background: section.config.dot }"></span>
          <span
            class="text-xs font-medium uppercase tracking-wide"
            :style="{ color: section.config.color }"
          >
            {{ section.config.label }} ({{ section.alerts.length }})
          </span>
        </div>
        <div class="space-y-1.5">
          <div
            v-for="alert in section.alerts"
            :key="alert.id"
            class="flex items-center justify-between rounded-md border px-3 py-2 cursor-pointer hover:brightness-110 transition-all"
            :style="{
              background: section.config.bg,
              borderColor: section.config.color,
            }"
            @click="openEntityDetail(alert)"
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span
                  class="rounded px-1.5 py-0.5 text-xs font-medium"
                  style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
                >
                  {{ alert.source }}
                </span>
                <span class="truncate text-sm" :style="{ color: section.config.color }">
                  {{ alert.message }}
                </span>
              </div>
              <div v-if="alert.entity_name" class="mt-0.5 text-xs" style="color: var(--pb-text-muted)">
                {{ alert.entity_name }}
              </div>
            </div>
            <span class="ml-3 shrink-0 text-xs" style="color: var(--pb-text-muted)">
              {{ timeAgo(alert.fired_at) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
