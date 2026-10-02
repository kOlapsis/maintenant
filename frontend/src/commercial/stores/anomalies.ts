// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  getAnomalySummary,
  listAnomalySeries,
  listAnomalyEvents,
  getAnomalySettings,
  updateAnomalySettings,
  type AnomalySummary,
  type AnomalySettings,
  type SeriesItem,
  type AnomalyEventItem,
} from '@/commercial/services/anomalyApi'
import { sseBus } from '@/services/sseBus'

export const useAnomaliesStore = defineStore('anomalies', () => {
  const summary = ref<AnomalySummary>({})
  const series = ref<SeriesItem[]>([])
  const events = ref<AnomalyEventItem[]>([])
  const settings = ref<AnomalySettings | null>(null)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)
  const savingSettings = ref(false)

  const containerSummary = computed(() => summary.value.container)
  const hostSummary = computed(() => summary.value.host)

  const anyReady = computed(() => series.value.some((s) => s.state === 'ready'))

  const activeEvents = computed(() =>
    events.value.filter((e) => e.ended_at == null).sort((a, b) => b.started_at - a.started_at),
  )

  function seriesKey(s: { scope_type: string; scope_id: string; metric: string; dimension: string }): string {
    return `${s.scope_type}|${s.scope_id}|${s.metric}|${s.dimension}`
  }

  async function load() {
    loading.value = true
    error.value = null
    try {
      const [sum, ser, evs, set] = await Promise.all([
        getAnomalySummary(),
        listAnomalySeries(),
        listAnomalyEvents({ limit: 200 }),
        getAnomalySettings(),
      ])
      summary.value = sum
      series.value = ser
      events.value = evs
      settings.value = set
      loaded.value = true
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load anomalies'
    } finally {
      loading.value = false
    }
  }

  function onOpened(e: MessageEvent) {
    let ev: AnomalyEventItem
    try {
      ev = JSON.parse(e.data)
    } catch {
      return
    }
    const existing = events.value.find((x) => x.id === ev.id)
    if (existing) Object.assign(existing, ev)
    else events.value = [ev, ...events.value]
  }

  function onClosed(e: MessageEvent) {
    let payload: { id: string; ended_at: number }
    try {
      payload = JSON.parse(e.data)
    } catch {
      return
    }
    const existing = events.value.find((x) => x.id === payload.id)
    if (existing) existing.ended_at = payload.ended_at
  }

  function onStateChanged(e: MessageEvent) {
    let payload: Partial<SeriesItem> & { scope_type: string; scope_id: string; metric: string; dimension: string }
    try {
      payload = JSON.parse(e.data)
    } catch {
      return
    }
    const existing = series.value.find((s) => seriesKey(s) === seriesKey(payload))
    if (existing) {
      Object.assign(existing, payload)
    } else if (payload.state) {
      series.value = [...series.value, payload as SeriesItem]
    }
  }

  function onReconnected() {
    if (loaded.value) load()
  }

  function connectSSE() {
    sseBus.on('anomaly.opened', onOpened)
    sseBus.on('anomaly.closed', onClosed)
    sseBus.on('anomaly.state_changed', onStateChanged)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('anomaly.opened', onOpened)
    sseBus.off('anomaly.closed', onClosed)
    sseBus.off('anomaly.state_changed', onStateChanged)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  async function saveBucketPull(value: number) {
    savingSettings.value = true
    try {
      settings.value = await updateAnomalySettings(value)
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to save settings'
    } finally {
      savingSettings.value = false
    }
  }

  function reset() {
    summary.value = {}
    series.value = []
    events.value = []
    settings.value = null
    loaded.value = false
    error.value = null
  }

  return {
    summary,
    series,
    events,
    settings,
    loading,
    loaded,
    error,
    savingSettings,
    containerSummary,
    hostSummary,
    anyReady,
    activeEvents,
    load,
    saveBucketPull,
    connectSSE,
    disconnectSSE,
    reset,
  }
})
