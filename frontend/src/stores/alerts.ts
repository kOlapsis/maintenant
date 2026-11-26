// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import {
  listAlerts,
  getActiveAlerts,
  listSilenceRules,
  acknowledgeAlert as ackAlert,
  type Alert,
  type SilenceRule,
  type ListAlertsParams,
} from '@/services/alertApi'
import { sseBus } from '@/services/sseBus'

// Identité envoyée au backend pour `acknowledged_by` (non vide requis).
// Placeholder constant tant qu'il n'y a pas d'auth/utilisateur dans l'app.
const ACK_ACTOR = 'operator'

export const useAlertsStore = defineStore('alerts', () => {
  // Alert state
  const alerts = ref<Alert[]>([])
  const activeAlerts = ref<{ critical: Alert[]; warning: Alert[]; info: Alert[] }>({
    critical: [],
    warning: [],
    info: [],
  })
  const hasMore = ref(false)
  const pinnedId = ref<string | null>(null)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const activeAlertsError = ref<string | null>(null)

  // Silence state
  const silenceRules = ref<SilenceRule[]>([])
  const silenceLoading = ref(false)

  // New alert counter
  const newAlertCount = ref(0)

  const totalActiveCount = computed(
    () => activeAlerts.value.critical.length + activeAlerts.value.warning.length + activeAlerts.value.info.length,
  )

  const activeSilenceCount = computed(() => silenceRules.value.filter((r) => r.is_active).length)

  let historyFilters: ListAlertsParams | undefined

  function inHistory(a: Alert): boolean {
    const f = historyFilters
    return (!f?.source || f.source === a.source) &&
      (!f?.severity || f.severity === a.severity) &&
      (!f?.status || f.status === a.status)
  }

  async function fetchAlerts(params?: ListAlertsParams) {
    if (!params?.before) historyFilters = params
    loading.value = true
    error.value = null
    try {
      const res = await listAlerts(params)
      if (params?.before) {
        alerts.value = [...alerts.value, ...res.alerts]
      } else {
        const pinned = alerts.value.find((a) => a.id === pinnedId.value)
        alerts.value = pinned && !res.alerts.some((a) => a.id === pinned.id) ? [pinned, ...res.alerts] : res.alerts
      }
      hasMore.value = res.has_more
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch alerts'
    } finally {
      loading.value = false
    }
  }

  let activeRequest = 0
  let appliedRequest = 0

  async function fetchActiveAlerts() {
    const request = ++activeRequest
    try {
      const res = await getActiveAlerts()
      if (request > appliedRequest) {
        appliedRequest = request
        activeAlerts.value = res
        activeAlertsError.value = null
      }
    } catch (e) {
      if (request === activeRequest) {
        activeAlertsError.value = e instanceof Error ? e.message : 'Failed to fetch active alerts'
      }
      console.error('Failed to fetch active alerts:', e)
    }
  }

  watch(
    () => sseBus.connected.value,
    (up) => {
      if (up && activeAlertsError.value) void fetchActiveAlerts()
    },
  )

  async function fetchSilenceRules(activeOnly = false) {
    silenceLoading.value = true
    try {
      const res = await listSilenceRules(activeOnly)
      silenceRules.value = res.rules
    } catch (e) {
      console.error('Failed to fetch silence rules:', e)
    } finally {
      silenceLoading.value = false
    }
  }

  async function acknowledgeAlert(id: string) {
    const updated = await ackAlert(id, ACK_ACTOR)
    // Mise à jour immédiate ; l'event SSE `alert.acknowledged` qui suivra
    // refait le même travail (idempotent).
    updateAlertInList(updated)
    upsertActive(updated)
    return updated
  }

  function clearNewAlertCount() {
    newAlertCount.value = 0
  }

  // SSE handlers
  function onAlertFired(e: MessageEvent) {
    let alert: Alert
    try {
      alert = JSON.parse(e.data)
    } catch {
      return
    }
    // Upsert by dedup key: if an alert for the same entity+type already
    // exists (e.g. severity escalation or same entity with new DB ID),
    // replace it instead of adding a duplicate.
    const key = alertKey(alert)
    const existingIdx = alerts.value.findIndex((a) => a.id === alert.id || (a.status === 'active' && alertKey(a) === key))
    if (existingIdx >= 0) {
      alerts.value[existingIdx] = alert
    } else {
      if (inHistory(alert)) alerts.value = [alert, ...alerts.value]
      newAlertCount.value++
    }
    upsertActive(alert)
  }

  function onAlertResolved(e: MessageEvent) {
    let alert: Alert
    try {
      alert = JSON.parse(e.data)
    } catch {
      return
    }
    updateAlertInList(alert)
    removeFromActive(alert.id)
  }

  function onAlertAcknowledged(e: MessageEvent) {
    let alert: Alert
    try {
      alert = JSON.parse(e.data)
    } catch {
      return
    }
    // Une alerte acquittée reste active : on la met à jour en place
    // (badge « Acquittée »), on ne la retire surtout pas de la liste active.
    updateAlertInList(alert)
    upsertActive(alert)
  }

  function onAlertSilenced(e: MessageEvent) {
    let alert: Alert
    try {
      alert = JSON.parse(e.data)
    } catch {
      return
    }
    const rest = alerts.value.filter((a) => a.id !== alert.id)
    alerts.value = inHistory(alert) ? [alert, ...rest] : rest
  }

  function onSilenceCreated(e: MessageEvent) {
    let rule: SilenceRule
    try {
      rule = JSON.parse(e.data)
    } catch {
      return
    }
    silenceRules.value = [rule, ...silenceRules.value]
  }

  function onSilenceCancelled(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = silenceRules.value.findIndex((r) => r.id === data.id)
    if (idx >= 0) {
      silenceRules.value[idx] = { ...silenceRules.value[idx]!, is_active: false, cancelled_at: new Date().toISOString() }
    }
  }

  function onReconnected() {
    fetchAlerts(historyFilters)
    fetchActiveAlerts()
    fetchSilenceRules()
  }

  function connectSSE() {
    sseBus.on('alert.fired', onAlertFired)
    sseBus.on('alert.resolved', onAlertResolved)
    sseBus.on('alert.acknowledged', onAlertAcknowledged)
    sseBus.on('alert.silenced', onAlertSilenced)
    sseBus.on('silence.created', onSilenceCreated)
    sseBus.on('silence.cancelled', onSilenceCancelled)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('alert.fired', onAlertFired)
    sseBus.off('alert.resolved', onAlertResolved)
    sseBus.off('alert.acknowledged', onAlertAcknowledged)
    sseBus.off('alert.silenced', onAlertSilenced)
    sseBus.off('silence.created', onSilenceCreated)
    sseBus.off('silence.cancelled', onSilenceCancelled)
    sseBus.off('sse.reconnected', onReconnected)
    sseBus.disconnect()
  }

  // Helpers

  // alertKey returns the dedup key for an alert (matches backend activeAlertKey).
  function alertKey(a: Alert): string {
    return `${a.source}/${a.alert_type}/${a.entity_type}/${a.entity_id}`
  }

  function upsertActive(alert: Alert) {
    if (alert.status !== 'active') return
    const key = alertKey(alert)
    // Remove any alert with the same dedup key from all severity buckets.
    // This handles escalation (warning→critical) and duplicate entity alerts.
    for (const sev of ['critical', 'warning', 'info'] as const) {
      activeAlerts.value[sev] = activeAlerts.value[sev].filter((a) => alertKey(a) !== key)
    }
    // Add to the correct bucket
    const severity = alert.severity as 'critical' | 'warning' | 'info'
    if (activeAlerts.value[severity]) {
      activeAlerts.value[severity] = [alert, ...activeAlerts.value[severity]]
    }
  }

  function removeFromActive(alertId: string) {
    for (const key of ['critical', 'warning', 'info'] as const) {
      activeAlerts.value[key] = activeAlerts.value[key].filter((a) => a.id !== alertId)
    }
  }

  function pinAlert(alert: Alert) {
    pinnedId.value = alert.id
    if (!alerts.value.some((a) => a.id === alert.id)) {
      alerts.value = [alert, ...alerts.value]
    }
  }

  function unpinAlert() {
    pinnedId.value = null
  }

  function updateAlertInList(updated: Alert) {
    const idx = alerts.value.findIndex((a) => a.id === updated.id)
    if (idx >= 0) {
      alerts.value[idx] = updated
    }
  }

  return {
    alerts,
    activeAlerts,
    hasMore,
    loading,
    error,
    activeAlertsError,
    silenceRules,
    silenceLoading,
    newAlertCount,
    totalActiveCount,
    activeSilenceCount,
    fetchAlerts,
    fetchActiveAlerts,
    retryActiveAlerts: fetchActiveAlerts,
    fetchSilenceRules,
    acknowledgeAlert,
    pinAlert,
    unpinAlert,
    clearNewAlertCount,
    connectSSE,
    disconnectSSE,
  }
})
