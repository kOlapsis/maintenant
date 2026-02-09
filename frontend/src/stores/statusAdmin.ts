import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  listGroups,
  listComponents,
  listIncidents,
  listMaintenance,
  listSubscribers,
  type ComponentGroup,
  type StatusComponent,
  type Incident,
  type MaintenanceWindow,
  type MaskedSubscriber,
} from '@/services/statusApi'
import { sseBus } from '@/services/sseBus'
import { useEdition } from '@/composables/useEdition'

export const useStatusAdminStore = defineStore('statusAdmin', () => {
  const { hasFeature } = useEdition()
  // Groups
  const groups = ref<ComponentGroup[]>([])
  const groupsLoading = ref(false)

  // Components
  const components = ref<StatusComponent[]>([])
  const componentsLoading = ref(false)

  // Incidents
  const incidents = ref<Incident[]>([])
  const incidentsTotal = ref(0)
  const incidentsLoading = ref(false)

  // Maintenance
  const maintenance = ref<MaintenanceWindow[]>([])
  const maintenanceLoading = ref(false)

  // Subscribers
  const subscribers = ref<MaskedSubscriber[]>([])
  const subscriberTotal = ref(0)
  const subscriberConfirmed = ref(0)
  const subscribersLoading = ref(false)

  const error = ref<string | null>(null)

  async function fetchGroups() {
    groupsLoading.value = true
    try {
      groups.value = await listGroups()
    } catch (e) {
      console.error('Failed to fetch groups:', e)
    } finally {
      groupsLoading.value = false
    }
  }

  async function fetchComponents() {
    componentsLoading.value = true
    try {
      components.value = await listComponents()
    } catch (e) {
      console.error('Failed to fetch components:', e)
    } finally {
      componentsLoading.value = false
    }
  }

  async function fetchIncidents(params?: { status?: string; severity?: string; limit?: number; offset?: number }) {
    if (!hasFeature('incidents')) return
    incidentsLoading.value = true
    try {
      const res = await listIncidents(params)
      incidents.value = res.incidents ?? []
      incidentsTotal.value = res.total ?? 0
    } catch (e) {
      console.error('Failed to fetch incidents:', e)
    } finally {
      incidentsLoading.value = false
    }
  }

  async function fetchMaintenance(params?: { status?: string; limit?: number }) {
    if (!hasFeature('maintenance_windows')) return
    maintenanceLoading.value = true
    try {
      maintenance.value = await listMaintenance(params)
    } catch (e) {
      console.error('Failed to fetch maintenance:', e)
    } finally {
      maintenanceLoading.value = false
    }
  }

  async function fetchSubscribers() {
    if (!hasFeature('subscribers')) return
    subscribersLoading.value = true
    try {
      const res = await listSubscribers()
      subscribers.value = res.subscribers
      subscriberTotal.value = res.total
      subscriberConfirmed.value = res.confirmed
    } catch (e) {
      console.error('Failed to fetch subscribers:', e)
    } finally {
      subscribersLoading.value = false
    }
  }

  // SSE handlers
  function onComponentChanged() {
    fetchComponents()
  }

  function onIncidentCreated() {
    if (!hasFeature('incidents')) return
    fetchIncidents()
  }

  function onIncidentUpdated() {
    if (!hasFeature('incidents')) return
    fetchIncidents()
  }

  function onIncidentResolved() {
    if (!hasFeature('incidents')) return
    fetchIncidents()
  }

  function onMaintenanceStarted() {
    if (!hasFeature('maintenance_windows')) return
    fetchMaintenance()
  }

  function onMaintenanceEnded() {
    if (!hasFeature('maintenance_windows')) return
    fetchMaintenance()
  }

  function connectSSE() {
    sseBus.on('status.component_changed', onComponentChanged)
    sseBus.on('status.incident_created', onIncidentCreated)
    sseBus.on('status.incident_updated', onIncidentUpdated)
    sseBus.on('status.incident_resolved', onIncidentResolved)
    sseBus.on('status.maintenance_started', onMaintenanceStarted)
    sseBus.on('status.maintenance_ended', onMaintenanceEnded)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('status.component_changed', onComponentChanged)
    sseBus.off('status.incident_created', onIncidentCreated)
    sseBus.off('status.incident_updated', onIncidentUpdated)
    sseBus.off('status.incident_resolved', onIncidentResolved)
    sseBus.off('status.maintenance_started', onMaintenanceStarted)
    sseBus.off('status.maintenance_ended', onMaintenanceEnded)
    sseBus.disconnect()
  }

  return {
    groups,
    groupsLoading,
    components,
    componentsLoading,
    incidents,
    incidentsTotal,
    incidentsLoading,
    maintenance,
    maintenanceLoading,
    subscribers,
    subscriberTotal,
    subscriberConfirmed,
    subscribersLoading,
    error,
    fetchGroups,
    fetchComponents,
    fetchIncidents,
    fetchMaintenance,
    fetchSubscribers,
    connectSSE,
    disconnectSSE,
  }
})
