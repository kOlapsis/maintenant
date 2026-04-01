// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  listAlerts,
  getActiveAlerts,
  listChannels,
  listSilenceRules,
  type Alert,
  type NotificationChannel,
  type SilenceRule,
  type ListAlertsParams,
} from '@/services/alertApi'
import { sseBus } from '@/services/sseBus'

export const useAlertsStore = defineStore('alerts', () => {
  // Alert state
  const alerts = ref<Alert[]>([])
  const activeAlerts = ref<{ critical: Alert[]; warning: Alert[]; info: Alert[] }>({
    critical: [],
    warning: [],
    info: [],
  })
  const hasMore = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Channel state
  const channels = ref<NotificationChannel[]>([])
  const channelsLoading = ref(false)

  // Silence state
  const silenceRules = ref<SilenceRule[]>([])
  const silenceLoading = ref(false)

  // New alert counter
  const newAlertCount = ref(0)

  const totalActiveCount = computed(
    () => activeAlerts.value.critical.length + activeAlerts.value.warning.length + activeAlerts.value.info.length,
  )

  const activeSilenceCount = computed(() => silenceRules.value.filter((r) => r.is_active).length)

  async function fetchAlerts(params?: ListAlertsParams) {
    loading.value = true
    error.value = null
    try {
      const res = await listAlerts(params)
      if (params?.before) {
        alerts.value = [...alerts.value, ...res.alerts]
      } else {
        alerts.value = res.alerts
      }
      hasMore.value = res.has_more
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch alerts'
    } finally {
      loading.value = false
    }
  }

  async function fetchActiveAlerts() {
    try {
      activeAlerts.value = await getActiveAlerts()
    } catch (e) {
      console.error('Failed to fetch active alerts:', e)
    }
  }

  async function fetchChannels() {
    channelsLoading.value = true
    try {
      const res = await listChannels()
      channels.value = res.channels
    } catch (e) {
      console.error('Failed to fetch channels:', e)
    } finally {
      channelsLoading.value = false
    }
  }

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
      alerts.value = [alert, ...alerts.value]
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
    updateAlertInList(alert)
    removeFromActive(alert.id)
  }

  function onAlertSilenced(e: MessageEvent) {
    let alert: Alert
    try {
      alert = JSON.parse(e.data)
    } catch {
      return
    }
    alerts.value = [alert, ...alerts.value]
  }

  function onChannelCreated(e: MessageEvent) {
    let channel: NotificationChannel
    try {
      channel = JSON.parse(e.data)
    } catch {
      return
    }
    channels.value = [...channels.value, channel]
  }

  function onChannelUpdated(e: MessageEvent) {
    let channel: NotificationChannel
    try {
      channel = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = channels.value.findIndex((c) => c.id === channel.id)
    if (idx >= 0) channels.value[idx] = channel
  }

  function onChannelDeleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    channels.value = channels.value.filter((c) => c.id !== data.id)
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
    fetchActiveAlerts()
    fetchChannels()
  }

  function connectSSE() {
    sseBus.on('alert.fired', onAlertFired)
    sseBus.on('alert.resolved', onAlertResolved)
    sseBus.on('alert.acknowledged', onAlertAcknowledged)
    sseBus.on('alert.silenced', onAlertSilenced)
    sseBus.on('channel.created', onChannelCreated)
    sseBus.on('channel.updated', onChannelUpdated)
    sseBus.on('channel.deleted', onChannelDeleted)
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
    sseBus.off('channel.created', onChannelCreated)
    sseBus.off('channel.updated', onChannelUpdated)
    sseBus.off('channel.deleted', onChannelDeleted)
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

  function removeFromActive(alertId: number) {
    for (const key of ['critical', 'warning', 'info'] as const) {
      activeAlerts.value[key] = activeAlerts.value[key].filter((a) => a.id !== alertId)
    }
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
    channels,
    channelsLoading,
    silenceRules,
    silenceLoading,
    newAlertCount,
    totalActiveCount,
    activeSilenceCount,
    fetchAlerts,
    fetchActiveAlerts,
    fetchChannels,
    fetchSilenceRules,
    clearNewAlertCount,
    connectSSE,
    disconnectSSE,
  }
})
