<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed, inject, onUnmounted, ref, watch } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { timeAgo } from '@/utils/time'
import type { Alert } from '@/services/alertApi'
import { humanizeAlertType } from '@/utils/alertLabels'
import EscalationStatusBadge from '@/commercial/components/escalation/EscalationStatusBadge.vue'
import AcknowledgeButton from '@/components/ui/AcknowledgeButton.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { useEscalationApi } from '@/commercial/composables/useEscalationApi'
import { useEdition } from '@/composables/useEdition'
import type { EscalationRun } from '@/commercial/types/escalation'

const detailSlideOver = inject(detailSlideOverKey)!
const store = useAlertsStore()

const escalationApi = useEscalationApi()
const { hasFeature } = useEdition()
const escalationRuns = ref<Record<string, EscalationRun[]>>({})

const retries = new Map<string, { delay: number; timer: ReturnType<typeof setTimeout> }>()

async function loadEscalationRuns(alertId: string) {
  try {
    const { runs } = await escalationApi.listRunsForAlert(alertId)
    escalationRuns.value = { ...escalationRuns.value, [alertId]: runs }
    const shown = runs.find((r) => r.status === 'active') ?? runs[0]
    if (shown) {
      const detail = await escalationApi.getEscalationRun(shown.id)
      escalationRuns.value = { ...escalationRuns.value, [alertId]: runs.map((r) => (r.id === shown.id ? detail : r)) }
    }
    retries.delete(alertId)
  } catch {
    const delay = retries.get(alertId)?.delay ?? 30_000
    const timer = setTimeout(() => {
      if (loadedAck.has(alertId)) void loadEscalationRuns(alertId)
    }, delay)
    retries.set(alertId, { delay: Math.min(delay * 2, 300_000), timer })
  }
}

onUnmounted(() => {
  for (const { timer } of retries.values()) clearTimeout(timer)
})

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

const severityConfig: Record<string, { label: string; color: string; bg: string; dot: string }> = {
  critical: {
    label: 'Critical',
    color: 'var(--mnt-status-down)',
    bg: 'var(--mnt-status-down-bg)',
    dot: 'var(--mnt-status-down)',
  },
  warning: {
    label: 'Warning',
    color: 'var(--mnt-status-warn)',
    bg: 'var(--mnt-status-warn-bg)',
    dot: 'var(--mnt-status-warn)',
  },
  info: {
    label: 'Info',
    color: 'var(--mnt-accent)',
    bg: 'rgba(59, 130, 246, 0.15)',
    dot: 'var(--mnt-accent)',
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

const loadedAck = new Map<string, string>()

watch(
  () =>
    hasFeature('alert_escalation')
      ? (Object.values(store.activeAlerts).flat() as Alert[]).map((a) => [a.id, a.acknowledged_at ?? ''] as const)
      : [],
  (alerts) => {
    for (const id of [...loadedAck.keys()]) {
      if (!alerts.some(([active]) => active === id)) loadedAck.delete(id)
    }
    for (const [id, ack] of alerts) {
      if (loadedAck.get(id) === ack) continue
      loadedAck.set(id, ack)
      loadEscalationRuns(id)
    }
  },
  { immediate: true },
)
</script>

<template>
  <div>
    <ErrorState
      v-if="store.activeAlertsError"
      title="Active alerts unavailable"
      :message="store.activeAlertsError"
      retryable
      @retry="store.retryActiveAlerts()"
    />

    <div
      v-if="store.totalActiveCount === 0 && !store.activeAlertsError"
      class="rounded-lg border p-6 text-center"
      style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
    >
      <p class="text-sm" style="color: var(--mnt-text-muted)">No active alerts</p>
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
                  style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
                >
                  {{ alert.source }}
                </span>
                <span class="truncate text-sm" :style="{ color: section.config.color }">
                  {{ alertTitle(alert) }}
                </span>
                <EscalationStatusBadge
                  v-if="escalationRuns[alert.id]?.length"
                  :runs="escalationRuns[alert.id] ?? []"
                />
              </div>
              <div v-if="alert.entity_name" class="mt-0.5 text-xs" style="color: var(--mnt-text-muted)">
                {{ alert.entity_name }}
              </div>
            </div>
            <div class="ml-3 flex shrink-0 items-center gap-2">
              <AcknowledgeButton :alert="alert" />
              <span class="text-xs" style="color: var(--mnt-text-muted)">
                {{ timeAgo(alert.fired_at) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
