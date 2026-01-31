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
    alerts.value = [alert, ...alerts.value]
    addToActiveByKey(alert)
    newAlertCount.value++
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

  function connectSSE() {
    sseBus.on('alert.fired', onAlertFired)
    sseBus.on('alert.resolved', onAlertResolved)
    sseBus.on('alert.silenced', onAlertSilenced)
    sseBus.on('channel.created', onChannelCreated)
    sseBus.on('channel.updated', onChannelUpdated)
    sseBus.on('channel.deleted', onChannelDeleted)
    sseBus.on('silence.created', onSilenceCreated)
    sseBus.on('silence.cancelled', onSilenceCancelled)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('alert.fired', onAlertFired)
    sseBus.off('alert.resolved', onAlertResolved)
    sseBus.off('alert.silenced', onAlertSilenced)
    sseBus.off('channel.created', onChannelCreated)
    sseBus.off('channel.updated', onChannelUpdated)
    sseBus.off('channel.deleted', onChannelDeleted)
    sseBus.off('silence.created', onSilenceCreated)
    sseBus.off('silence.cancelled', onSilenceCancelled)
    sseBus.disconnect()
  }

  // Helpers
  function addToActiveByKey(alert: Alert) {
    if (alert.status !== 'active') return
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
