import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  fetchUpdates,
  fetchUpdateSummary,
  triggerScan,
  fetchContainerUpdate,
  fetchCves,
  fetchContainerCves,
  fetchRiskScores,
  fetchRiskHistory,
  pinVersion as apiPinVersion,
  unpinVersion as apiUnpinVersion,
  fetchExclusions,
  addExclusion as apiAddExclusion,
  removeExclusion as apiRemoveExclusion,
  type ImageUpdate,
  type UpdateSummary,
  type CVEInfo,
  type RiskListResponse,
  type Exclusion,
} from '@/services/updateApi'
import { sseBus } from '@/services/sseBus'
import { useEdition } from '@/composables/useEdition'

export const useUpdatesStore = defineStore('updates', () => {
  const { hasFeature } = useEdition()
  const updates = ref<ImageUpdate[]>([])
  const summary = ref<UpdateSummary | null>(null)
  const scanning = ref(false)
  const loading = ref(false)
  const containerCves = ref<Record<string, CVEInfo[]>>({})
  const riskScores = ref<RiskListResponse | null>(null)
  const exclusions = ref<Exclusion[]>([])
  const lastScan = ref<string>('')
  const nextScan = ref<string>('')

  const criticalCount = computed(() => summary.value?.counts?.critical ?? 0)
  const recommendedCount = computed(() => summary.value?.counts?.recommended ?? 0)
  const availableCount = computed(() => summary.value?.counts?.available ?? 0)
  const totalUpdateCount = computed(() => criticalCount.value + recommendedCount.value + availableCount.value)

  // SSE handlers
  function onScanStarted() {
    scanning.value = true
  }

  function onScanCompleted(e: MessageEvent) {
    scanning.value = false
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    if (data.updates_found > 0) {
      fetchSummary()
      fetchAllUpdates()
    }
  }

  function onDetected(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = updates.value.findIndex(u => u.container_id === data.container_id)
    if (idx >= 0) {
      updates.value[idx] = { ...updates.value[idx], ...data }
    }
  }

  function onPinned(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = updates.value.findIndex(u => u.container_id === data.container_id)
    if (idx >= 0) {
      updates.value[idx].status = 'pinned'
    }
  }

  function onUnpinned(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = updates.value.findIndex(u => u.container_id === data.container_id)
    if (idx >= 0) {
      updates.value[idx].status = 'available'
    }
  }

  function onCveDetected() {
    if (!hasFeature('cve_enrichment')) return
    fetchSummary()
  }

  function onRiskUpdated() {
    if (!hasFeature('risk_scoring')) return
    fetchSummary()
  }

  function connectSSE() {
    sseBus.on('update.scan_started', onScanStarted)
    sseBus.on('update.scan_completed', onScanCompleted)
    sseBus.on('update.detected', onDetected)
    sseBus.on('update.pinned', onPinned)
    sseBus.on('update.unpinned', onUnpinned)
    sseBus.on('cve.detected', onCveDetected)
    sseBus.on('risk.updated', onRiskUpdated)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('update.scan_started', onScanStarted)
    sseBus.off('update.scan_completed', onScanCompleted)
    sseBus.off('update.detected', onDetected)
    sseBus.off('update.pinned', onPinned)
    sseBus.off('update.unpinned', onUnpinned)
    sseBus.off('cve.detected', onCveDetected)
    sseBus.off('risk.updated', onRiskUpdated)
    sseBus.disconnect()
  }

  async function fetchAllUpdates(filters?: { status?: string; update_type?: string; min_risk?: number }) {
    loading.value = true
    try {
      const data = await fetchUpdates(filters)
      updates.value = data.updates || []
      lastScan.value = data.last_scan
      nextScan.value = data.next_scan
    } catch {
      // ignore
    } finally {
      loading.value = false
    }
  }

  async function fetchSummary() {
    try {
      summary.value = await fetchUpdateSummary()
    } catch {
      // ignore
    }
  }

  async function startScan() {
    try {
      scanning.value = true
      await triggerScan()
      // Poll summary until scan_status changes (fallback if SSE event is missed)
      const poll = setInterval(async () => {
        try {
          const s = await fetchUpdateSummary()
          if (s.scan_status !== 'scanning') {
            scanning.value = false
            summary.value = s
            await fetchAllUpdates()
            clearInterval(poll)
          }
        } catch {
          clearInterval(poll)
          scanning.value = false
        }
      }, 3000)
      // Safety timeout — stop polling after 2 minutes
      setTimeout(() => { clearInterval(poll); scanning.value = false }, 120_000)
    } catch {
      scanning.value = false
    }
  }

  async function fetchContainerCvesAction(containerId: string) {
    if (!hasFeature('cve_enrichment')) return
    try {
      const data = await fetchContainerCves(containerId)
      containerCves.value[containerId] = data.cves || []
    } catch {
      // ignore
    }
  }

  async function fetchRiskScoresAction() {
    if (!hasFeature('risk_scoring')) return
    try {
      riskScores.value = await fetchRiskScores()
    } catch {
      // ignore
    }
  }

  async function pinVersionAction(containerId: string, reason?: string) {
    try {
      await apiPinVersion(containerId, reason)
      const idx = updates.value.findIndex(u => u.container_id === containerId)
      if (idx >= 0) updates.value[idx].status = 'pinned'
      await fetchSummary()
    } catch {
      // ignore
    }
  }

  async function unpinVersionAction(containerId: string) {
    try {
      await apiUnpinVersion(containerId)
      const idx = updates.value.findIndex(u => u.container_id === containerId)
      if (idx >= 0) updates.value[idx].status = 'available'
      await fetchSummary()
    } catch {
      // ignore
    }
  }

  async function fetchExclusionsAction() {
    try {
      const data = await fetchExclusions()
      exclusions.value = data.exclusions || []
    } catch {
      // ignore
    }
  }

  async function addExclusionAction(pattern: string, patternType: string) {
    try {
      await apiAddExclusion(pattern, patternType)
      await fetchExclusionsAction()
    } catch {
      // ignore
    }
  }

  async function removeExclusionAction(id: number) {
    try {
      await apiRemoveExclusion(id)
      await fetchExclusionsAction()
    } catch {
      // ignore
    }
  }

  return {
    updates,
    summary,
    scanning,
    loading,
    containerCves,
    riskScores,
    exclusions,
    lastScan,
    nextScan,
    criticalCount,
    recommendedCount,
    availableCount,
    totalUpdateCount,
    connectSSE,
    disconnectSSE,
    fetchAllUpdates,
    fetchSummary,
    startScan,
    fetchContainerCves: fetchContainerCvesAction,
    fetchRiskScores: fetchRiskScoresAction,
    pinVersion: pinVersionAction,
    unpinVersion: unpinVersionAction,
    fetchExclusions: fetchExclusionsAction,
    addExclusion: addExclusionAction,
    removeExclusion: removeExclusionAction,
  }
})
