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
import { ref } from 'vue'
import {
  fetchWorkloads,
  fetchPods,
  fetchClusterOverview,
  fetchNodes,
  type K8sWorkloadGroup,
  type K8sPod,
  type K8sClusterOverview,
  type K8sNodeResponse,
} from '@/services/kubernetesApi'
import { useNamespacesStore } from '@/stores/namespaces'
import { useResourcesStore } from '@/stores/resources'
import { sseBus } from '@/services/sseBus'

export const useKubernetesStore = defineStore('kubernetes', () => {
  const workloadGroups = ref<K8sWorkloadGroup[]>([])
  const pods = ref<K8sPod[]>([])
  const clusterOverview = ref<K8sClusterOverview | null>(null)
  const nodes = ref<K8sNodeResponse[]>([])
  const loading = ref(false)
  const nodesLoading = ref(false)
  const clusterLoading = ref(false)
  const error = ref<string | null>(null)

  async function fetchWorkloadsList(params?: { kind?: string; status?: string }) {
    const namespacesStore = useNamespacesStore()
    loading.value = true
    error.value = null
    try {
      const resp = await fetchWorkloads({
        namespaces: namespacesStore.namespacesParam || undefined,
        kind: params?.kind,
        status: params?.status,
        agentId: useResourcesStore().entityQuery,
      })
      workloadGroups.value = resp.groups
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load workloads'
    } finally {
      loading.value = false
    }
  }

  async function fetchPodsList(params?: { workload?: string; node?: string; status?: string }) {
    const namespacesStore = useNamespacesStore()
    loading.value = true
    error.value = null
    try {
      const resp = await fetchPods({
        namespaces: namespacesStore.namespacesParam || undefined,
        workload: params?.workload,
        node: params?.node,
        status: params?.status,
        agentId: useResourcesStore().entityQuery,
      })
      pods.value = resp.pods
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load pods'
    } finally {
      loading.value = false
    }
  }

  async function fetchCluster() {
    clusterLoading.value = true
    error.value = null
    try {
      clusterOverview.value = await fetchClusterOverview(useResourcesStore().entityQuery)
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load cluster overview'
    } finally {
      clusterLoading.value = false
    }
  }

  async function fetchNodesList() {
    nodesLoading.value = true
    error.value = null
    try {
      const resp = await fetchNodes(useResourcesStore().entityQuery)
      nodes.value = resp.nodes
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load nodes'
    } finally {
      nodesLoading.value = false
    }
  }

  function onWorkloadChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { id: string; namespace: string }
      if (data.namespace) {
        fetchWorkloadsList()
      }
    } catch {
      /* ignore */
    }
  }

  function onPodChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { namespace: string; name: string }
      if (data.namespace) {
        fetchPodsList()
      }
    } catch {
      /* ignore */
    }
  }

  function onNodeChanged(_e: MessageEvent) {
    fetchNodesList()
    // Also refresh cluster overview since node changes affect health.
    if (clusterOverview.value !== null) {
      fetchCluster()
    }
  }

  // topologyMatchesScope reports whether a remote agent's topology event is
  // relevant to the current host scope. 'local' uses the runtime's own informer
  // events (workload/pod/node_changed), so it ignores topology_changed.
  function topologyMatchesScope(agentId: string): boolean {
    const sel = useResourcesStore().selected
    if (sel === null) return true
    if (sel === 'local') return false
    return sel === agentId
  }

  // onTopologyChanged handles a remote agent's full-snapshot reconcile: refetch
  // the lists currently in view for the matching scope.
  function onTopologyChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { agent_id: string }
      if (!topologyMatchesScope(data.agent_id)) return
      fetchWorkloadsList()
      fetchPodsList()
      if (nodes.value.length > 0) fetchNodesList()
      if (clusterOverview.value !== null) fetchCluster()
    } catch {
      /* ignore */
    }
  }

  function startListening() {
    sseBus.on('kubernetes.workload_changed', onWorkloadChanged)
    sseBus.on('kubernetes.pod_changed', onPodChanged)
    sseBus.on('kubernetes.node_changed', onNodeChanged)
    sseBus.on('kubernetes.topology_changed', onTopologyChanged)
  }

  function stopListening() {
    sseBus.off('kubernetes.workload_changed', onWorkloadChanged)
    sseBus.off('kubernetes.pod_changed', onPodChanged)
    sseBus.off('kubernetes.node_changed', onNodeChanged)
    sseBus.off('kubernetes.topology_changed', onTopologyChanged)
  }

  return {
    workloadGroups,
    pods,
    clusterOverview,
    nodes,
    loading,
    nodesLoading,
    clusterLoading,
    error,
    fetchWorkloadsList,
    fetchPodsList,
    fetchCluster,
    fetchNodesList,
    startListening,
    stopListening,
  }
})
