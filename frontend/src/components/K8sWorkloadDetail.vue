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
import { ref, onMounted } from 'vue'
import {
  fetchWorkloadDetail,
  fetchWorkloadResources,
  type K8sWorkloadDetailResponse,
  type K8sPodResourceEntry,
} from '@/services/kubernetesApi'
import FeatureGate from './FeatureGate.vue'
import { timeAgo } from '@/utils/time'
import { useEdition } from '@/composables/useEdition'

const props = defineProps<{
  workloadId: string
}>()

defineEmits<{
  close: []
}>()

type Tab = 'pods' | 'events' | 'conditions' | 'resources'

const { hasFeature } = useEdition()
const detail = ref<K8sWorkloadDetailResponse | null>(null)
const loading = ref(true)
const activeTab = ref<Tab>('pods')
const podResources = ref<K8sPodResourceEntry[]>([])
const metricsAvailable = ref(true)
const metricsMessage = ref('')
const resourcesLoading = ref(false)

const baseTabs: { key: Tab; label: string }[] = [
  { key: 'pods', label: 'Pods' },
  { key: 'events', label: 'Events' },
  { key: 'conditions', label: 'Conditions' },
]

const tabs = hasFeature('k8s_cluster')
  ? [...baseTabs, { key: 'resources' as Tab, label: 'Resources' }]
  : baseTabs

onMounted(async () => {
  try {
    detail.value = await fetchWorkloadDetail(props.workloadId)
  } finally {
    loading.value = false
  }
})

async function loadResources() {
  if (podResources.value.length > 0 || !metricsAvailable.value) return
  resourcesLoading.value = true
  try {
    const resp = await fetchWorkloadResources(props.workloadId)
    metricsAvailable.value = resp.metrics_available
    metricsMessage.value = resp.message ?? ''
    podResources.value = resp.pods
  } catch {
    metricsAvailable.value = false
    metricsMessage.value = 'Failed to load resource metrics'
  } finally {
    resourcesLoading.value = false
  }
}

function onTabClick(tab: Tab) {
  activeTab.value = tab
  if (tab === 'resources') loadResources()
}

function formatBytes(bytes: number | null): string {
  if (bytes === null || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function cpuDisplay(milli: number | null): string {
  if (milli === null) return '-'
  if (milli < 1000) return `${milli}m`
  return `${(milli / 1000).toFixed(2)} cores`
}

function memBarWidth(percent: number | null): string {
  if (percent === null) return '0%'
  return `${Math.min(percent, 100)}%`
}

function memColor(percent: number | null): string {
  if (percent === null) return 'bg-slate-700'
  if (percent > 85) return 'bg-red-500'
  if (percent > 60) return 'bg-amber-500'
  return 'bg-sky-500'
}

function statusStyle(status: string): string {
  switch (status) {
    case 'healthy': return 'text-pb-status-ok bg-pb-status-ok border-emerald-400/20'
    case 'degraded': return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
    case 'progressing': return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
    case 'failed': return 'text-red-400 bg-red-400/10 border-red-400/20'
    default: return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
  }
}

function podStatusStyle(status: string): string {
  const s = status.toLowerCase()
  if (s === 'running') return 'text-pb-status-ok bg-pb-status-ok border-emerald-400/20'
  if (s === 'pending') return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
  if (s === 'succeeded') return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
  if (s === 'failed') return 'text-red-400 bg-red-400/10 border-red-400/20'
  return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
}

function conditionStatusStyle(status: string): string {
  if (status === 'True') return 'text-pb-status-ok bg-pb-status-ok border-emerald-400/20'
  if (status === 'False') return 'text-red-400 bg-red-400/10 border-red-400/20'
  return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
}

function eventTypeStyle(type: string): string {
  if (type === 'Warning') return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
  return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
}

function replicaColor(ready: number, desired: number): string {
  if (ready >= desired && desired > 0) return 'text-pb-status-ok'
  if (ready > 0) return 'text-amber-400'
  return 'text-red-400'
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-16">
      <span class="text-sm text-slate-500">Loading workload…</span>
    </div>

    <template v-else-if="detail">
      <!-- Header -->
      <div class="px-5 pt-4 pb-3 border-b border-slate-800">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h2 class="text-base font-bold text-pb-primary truncate">{{ detail.workload.name }}</h2>
            <div class="flex items-center gap-2 mt-1 flex-wrap">
              <span class="text-[10px] font-bold uppercase tracking-wider text-sky-400 bg-sky-400/10 border border-sky-400/20 px-1.5 py-0.5 rounded">
                {{ detail.workload.kind }}
              </span>
              <span class="text-[10px] font-mono text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
                {{ detail.workload.namespace }}
              </span>
              <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', statusStyle(detail.workload.status)]">
                {{ detail.workload.status }}
              </span>
              <span :class="['text-xs font-semibold tabular-nums', replicaColor(detail.workload.ready_replicas, detail.workload.desired_replicas)]">
                {{ detail.workload.ready_replicas }}/{{ detail.workload.desired_replicas }} ready
              </span>
            </div>
          </div>
        </div>

        <!-- Images -->
        <div v-if="detail.workload.images.length > 0" class="mt-3">
          <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">
            Image{{ detail.workload.images.length > 1 ? 's' : '' }}
          </p>
          <div class="space-y-1">
            <div
              v-for="image in detail.workload.images"
              :key="image"
              class="flex items-center gap-2"
            >
              <span class="text-xs text-pb-secondary font-mono truncate">{{ image }}</span>
            </div>
          </div>
        </div>

        <!-- Timestamps -->
        <div class="mt-3 flex gap-6 text-xs text-slate-500">
          <span>Created <span class="text-slate-400">{{ timeAgo(detail.workload.created_at) }}</span></span>
          <span>Updated <span class="text-slate-400">{{ timeAgo(detail.workload.last_transition) }}</span></span>
        </div>
      </div>

      <!-- Tabs -->
      <div class="flex border-b border-slate-800 px-5">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          :class="[
            'px-4 py-2.5 text-xs font-bold uppercase tracking-widest border-b-2 -mb-px transition-colors',
            activeTab === tab.key
              ? 'border-pb-green-400 text-pb-green-400'
              : 'border-transparent text-slate-500 hover:text-pb-secondary',
          ]"
          @click="onTabClick(tab.key)"
        >
          {{ tab.label }}
          <span
            v-if="tab.key === 'pods'"
            class="ml-1 text-[10px] text-slate-600"
          >{{ detail.pods.length }}</span>
          <span
            v-if="tab.key === 'events'"
            class="ml-1 text-[10px] text-slate-600"
          >{{ detail.events.length }}</span>
          <span
            v-if="tab.key === 'conditions'"
            class="ml-1 text-[10px] text-slate-600"
          >{{ detail.workload.conditions.length }}</span>
        </button>
      </div>

      <!-- Tab content -->
      <div class="flex-1 overflow-y-auto px-5 py-4">
        <!-- Pods tab -->
        <template v-if="activeTab === 'pods'">
          <div v-if="detail.pods.length === 0" class="text-sm text-slate-500 py-4 text-center">
            No pods
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="pod in detail.pods"
              :key="`${pod.namespace}/${pod.name}`"
              class="bg-pb-primary rounded-lg border border-slate-800 px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-mono text-pb-primary truncate">{{ pod.name }}</span>
                <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0', podStatusStyle(pod.status)]">
                  {{ pod.status }}
                </span>
              </div>
              <div class="flex items-center gap-4 mt-1.5 text-xs text-slate-500">
                <span v-if="pod.node_name" class="font-mono">{{ pod.node_name }}</span>
                <span v-if="pod.pod_ip" class="font-mono">{{ pod.pod_ip }}</span>
                <span v-if="pod.restart_count > 0" class="text-amber-400">{{ pod.restart_count }}↺</span>
                <span>{{ timeAgo(pod.created_at) }}</span>
              </div>
            </div>
          </div>
        </template>

        <!-- Events tab -->
        <template v-else-if="activeTab === 'events'">
          <div v-if="detail.events.length === 0" class="text-sm text-slate-500 py-4 text-center">
            No events
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="(event, i) in detail.events"
              :key="i"
              class="bg-pb-primary rounded-lg border border-slate-800 px-4 py-3"
            >
              <div class="flex items-start gap-3">
                <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0 mt-0.5', eventTypeStyle(event.type)]">
                  {{ event.type }}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-semibold text-pb-secondary">{{ event.reason }}</span>
                    <span v-if="event.count > 1" class="text-[10px] text-slate-600 tabular-nums">×{{ event.count }}</span>
                  </div>
                  <p class="text-xs text-slate-400 mt-0.5 leading-relaxed">{{ event.message }}</p>
                  <div class="flex items-center gap-3 mt-1 text-xs text-slate-600">
                    <span v-if="event.source">{{ event.source }}</span>
                    <span>{{ timeAgo(event.last_seen) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>

        <!-- Conditions tab -->
        <template v-else-if="activeTab === 'conditions'">
          <div v-if="detail.workload.conditions.length === 0" class="text-sm text-slate-500 py-4 text-center">
            No conditions
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="condition in detail.workload.conditions"
              :key="condition.type"
              class="bg-pb-primary rounded-lg border border-slate-800 px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-semibold text-pb-primary">{{ condition.type }}</span>
                <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0', conditionStatusStyle(condition.status)]">
                  {{ condition.status }}
                </span>
              </div>
              <div v-if="condition.reason" class="mt-1 text-xs text-slate-400 font-semibold">
                {{ condition.reason }}
              </div>
              <p v-if="condition.message" class="mt-0.5 text-xs text-slate-500 leading-relaxed">
                {{ condition.message }}
              </p>
              <div class="mt-1 text-xs text-slate-600">
                {{ timeAgo(condition.last_transition) }}
              </div>
            </div>
          </div>
        </template>

        <!-- Resources tab (Enterprise) -->
        <template v-else-if="activeTab === 'resources'">
          <FeatureGate
            feature="k8s_cluster"
            title="Pod Resource Metrics"
            description="View per-pod CPU and memory usage from metrics-server."
          >
            <div v-if="resourcesLoading" class="text-sm text-slate-500 py-4 text-center">
              Loading resources...
            </div>
            <div v-else-if="!metricsAvailable" class="text-center py-8">
              <p class="text-sm text-slate-400 font-medium mb-1">Metrics unavailable</p>
              <p class="text-xs text-slate-500">{{ metricsMessage || 'Install metrics-server for resource data' }}</p>
            </div>
            <div v-else-if="podResources.length === 0" class="text-sm text-slate-500 py-4 text-center">
              No resource data available
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="pr in podResources"
                :key="`${pr.namespace}/${pr.name}`"
                class="bg-pb-primary rounded-lg border border-slate-800 px-4 py-3"
              >
                <div class="flex items-center justify-between mb-2">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-mono text-pb-primary truncate">{{ pr.name }}</span>
                    <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0', podStatusStyle(pr.status)]">
                      {{ pr.status }}
                    </span>
                  </div>
                  <span v-if="pr.node_name" class="text-xs text-slate-500 font-mono">{{ pr.node_name }}</span>
                </div>

                <!-- CPU -->
                <div class="mb-2">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">CPU</span>
                    <span class="text-xs text-slate-400 tabular-nums">{{ cpuDisplay(pr.cpu_millicores) }}</span>
                  </div>
                </div>

                <!-- Memory bar -->
                <div>
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Memory</span>
                    <span class="text-xs text-slate-400 tabular-nums">
                      {{ formatBytes(pr.mem_bytes) }}
                      <template v-if="pr.mem_limit_bytes"> / {{ formatBytes(pr.mem_limit_bytes) }}</template>
                      <span v-if="pr.mem_percent !== null" class="text-slate-600 ml-1">({{ pr.mem_percent.toFixed(1) }}%)</span>
                    </span>
                  </div>
                  <div v-if="pr.mem_limit_bytes" class="h-1.5 bg-pb-primary border border-slate-800 rounded-full overflow-hidden">
                    <div
                      :class="['h-full rounded-full transition-all', memColor(pr.mem_percent)]"
                      :style="{ width: memBarWidth(pr.mem_percent) }"
                    />
                  </div>
                </div>
              </div>
            </div>
          </FeatureGate>
        </template>
      </div>
    </template>

    <!-- Error / no data -->
    <div v-else class="flex items-center justify-center py-16">
      <span class="text-sm text-slate-500">Workload not found.</span>
    </div>
  </div>
</template>
