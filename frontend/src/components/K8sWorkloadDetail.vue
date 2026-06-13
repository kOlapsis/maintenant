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
import { useResourcesStore } from '@/stores/resources'

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
    detail.value = await fetchWorkloadDetail(props.workloadId, useResourcesStore().entityQuery)
  } finally {
    loading.value = false
  }
})

async function loadResources() {
  if (podResources.value.length > 0 || !metricsAvailable.value) return
  resourcesLoading.value = true
  try {
    const resp = await fetchWorkloadResources(props.workloadId, useResourcesStore().entityQuery)
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
  if (percent === null) return 'bg-mnt-elevated'
  if (percent > 85) return 'bg-mnt-sev-incident-solid'
  if (percent > 60) return 'bg-mnt-sev-warning-solid'
  return 'bg-mnt-sev-neutral-solid'
}

function statusStyle(status: string): string {
  switch (status) {
    case 'healthy':
      return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
    case 'degraded':
      return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
    case 'progressing':
      return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
    case 'failed':
      return 'text-mnt-status-down bg-mnt-status-down border-mnt-sev-incident'
    default:
      return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
  }
}

function podStatusStyle(status: string): string {
  const s = status.toLowerCase()
  if (s === 'running') return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
  if (s === 'pending') return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
  if (s === 'succeeded') return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
  if (s === 'failed') return 'text-mnt-status-down bg-mnt-status-down border-mnt-sev-incident'
  return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
}

function conditionStatusStyle(status: string): string {
  if (status === 'True') return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
  if (status === 'False') return 'text-mnt-status-down bg-mnt-status-down border-mnt-sev-incident'
  return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
}

function eventTypeStyle(type: string): string {
  if (type === 'Warning') return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
  return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
}

function replicaColor(ready: number, desired: number): string {
  if (ready >= desired && desired > 0) return 'text-mnt-status-ok'
  if (ready > 0) return 'text-mnt-status-warn'
  return 'text-mnt-status-down'
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-16">
      <span class="text-sm text-mnt-muted">Loading workload…</span>
    </div>

    <template v-else-if="detail">
      <!-- Header -->
      <div class="px-5 pt-4 mnt-3 border-b border-mnt-default">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <h2 class="text-base font-bold text-mnt-primary truncate">{{ detail.workload.name }}</h2>
            <div class="flex items-center gap-2 mt-1 flex-wrap">
              <span
                class="text-[10px] font-bold uppercase tracking-wider text-mnt-secondary bg-sky-400/10 border border-sky-400/20 px-1.5 py-0.5 rounded"
              >
                {{ detail.workload.kind }}
              </span>
              <span
                class="text-[10px] font-mono text-mnt-muted bg-mnt-elevated border border-mnt-default px-1.5 py-0.5 rounded"
              >
                {{ detail.workload.namespace }}
              </span>
              <span
                :class="[
                  'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border',
                  statusStyle(detail.workload.status),
                ]"
              >
                {{ detail.workload.status }}
              </span>
              <span
                :class="[
                  'text-xs font-semibold tabular-nums',
                  replicaColor(detail.workload.ready_replicas, detail.workload.desired_replicas),
                ]"
              >
                {{ detail.workload.ready_replicas }}/{{ detail.workload.desired_replicas }} ready
              </span>
            </div>
          </div>
        </div>

        <!-- Images -->
        <div v-if="detail.workload.images.length > 0" class="mt-3">
          <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">
            Image{{ detail.workload.images.length > 1 ? 's' : '' }}
          </p>
          <div class="space-y-1">
            <div
              v-for="image in detail.workload.images"
              :key="image"
              class="flex items-center gap-2"
            >
              <span class="text-xs text-mnt-secondary font-mono truncate">{{ image }}</span>
            </div>
          </div>
        </div>

        <!-- Timestamps -->
        <div class="mt-3 flex gap-6 text-xs text-mnt-muted">
          <span
            >Created
            <span class="text-mnt-muted">{{ timeAgo(detail.workload.created_at) }}</span></span
          >
          <span
            >Updated
            <span class="text-mnt-muted">{{ timeAgo(detail.workload.last_transition) }}</span></span
          >
        </div>
      </div>

      <!-- Tabs -->
      <div class="flex border-b border-mnt-default px-5">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          :class="[
            'px-4 py-2.5 text-xs font-bold uppercase tracking-widest border-b-2 -mb-px transition-colors',
            activeTab === tab.key
              ? 'border-mnt-green-400 text-mnt-green-400'
              : 'border-transparent text-mnt-muted hover:text-mnt-secondary',
          ]"
          @click="onTabClick(tab.key)"
        >
          {{ tab.label }}
          <span v-if="tab.key === 'pods'" class="ml-1 text-[10px] text-mnt-muted">{{
            detail.pods.length
          }}</span>
          <span v-if="tab.key === 'events'" class="ml-1 text-[10px] text-mnt-muted">{{
            detail.events.length
          }}</span>
          <span v-if="tab.key === 'conditions'" class="ml-1 text-[10px] text-mnt-muted">{{
            detail.workload.conditions.length
          }}</span>
        </button>
      </div>

      <!-- Tab content -->
      <div class="flex-1 overflow-y-auto px-5 py-4">
        <!-- Pods tab -->
        <template v-if="activeTab === 'pods'">
          <div v-if="detail.pods.length === 0" class="text-sm text-mnt-muted py-4 text-center">
            No pods
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="pod in detail.pods"
              :key="`${pod.namespace}/${pod.name}`"
              class="bg-mnt-primary rounded-lg border border-mnt-default px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-mono text-mnt-primary truncate">{{ pod.name }}</span>
                <span
                  :class="[
                    'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0',
                    podStatusStyle(pod.status),
                  ]"
                >
                  {{ pod.status }}
                </span>
              </div>
              <div class="flex items-center gap-4 mt-1.5 text-xs text-mnt-muted">
                <span v-if="pod.node_name" class="font-mono">{{ pod.node_name }}</span>
                <span v-if="pod.pod_ip" class="font-mono">{{ pod.pod_ip }}</span>
                <span v-if="pod.restart_count > 0" class="text-mnt-status-warn"
                  >{{ pod.restart_count }}↺</span
                >
                <span>{{ timeAgo(pod.created_at) }}</span>
              </div>
            </div>
          </div>
        </template>

        <!-- Events tab -->
        <template v-else-if="activeTab === 'events'">
          <div v-if="detail.events.length === 0" class="text-sm text-mnt-muted py-4 text-center">
            No events
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="(event, i) in detail.events"
              :key="i"
              class="bg-mnt-primary rounded-lg border border-mnt-default px-4 py-3"
            >
              <div class="flex items-start gap-3">
                <span
                  :class="[
                    'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0 mt-0.5',
                    eventTypeStyle(event.type),
                  ]"
                >
                  {{ event.type }}
                </span>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-semibold text-mnt-secondary">{{ event.reason }}</span>
                    <span v-if="event.count > 1" class="text-[10px] text-mnt-muted tabular-nums"
                      >×{{ event.count }}</span
                    >
                  </div>
                  <p class="text-xs text-mnt-muted mt-0.5 leading-relaxed">{{ event.message }}</p>
                  <div class="flex items-center gap-3 mt-1 text-xs text-mnt-muted">
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
          <div
            v-if="detail.workload.conditions.length === 0"
            class="text-sm text-mnt-muted py-4 text-center"
          >
            No conditions
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="condition in detail.workload.conditions"
              :key="condition.type"
              class="bg-mnt-primary rounded-lg border border-mnt-default px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-semibold text-mnt-primary">{{ condition.type }}</span>
                <span
                  :class="[
                    'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0',
                    conditionStatusStyle(condition.status),
                  ]"
                >
                  {{ condition.status }}
                </span>
              </div>
              <div v-if="condition.reason" class="mt-1 text-xs text-mnt-muted font-semibold">
                {{ condition.reason }}
              </div>
              <p v-if="condition.message" class="mt-0.5 text-xs text-mnt-muted leading-relaxed">
                {{ condition.message }}
              </p>
              <div class="mt-1 text-xs text-mnt-muted">
                {{ timeAgo(condition.last_transition) }}
              </div>
            </div>
          </div>
        </template>

        <!-- Resources tab (Pro) -->
        <template v-else-if="activeTab === 'resources'">
          <FeatureGate
            feature="k8s_cluster"
            title="Pod Resource Metrics"
            description="View per-pod CPU and memory usage from metrics-server."
          >
            <div v-if="resourcesLoading" class="text-sm text-mnt-muted py-4 text-center">
              Loading resources...
            </div>
            <div v-else-if="!metricsAvailable" class="text-center py-8">
              <p class="text-sm text-mnt-muted font-medium mb-1">Metrics unavailable</p>
              <p class="text-xs text-mnt-muted">
                {{ metricsMessage || 'Install metrics-server for resource data' }}
              </p>
            </div>
            <div
              v-else-if="podResources.length === 0"
              class="text-sm text-mnt-muted py-4 text-center"
            >
              No resource data available
            </div>
            <div v-else class="space-y-2">
              <div
                v-for="pr in podResources"
                :key="`${pr.namespace}/${pr.name}`"
                class="bg-mnt-primary rounded-lg border border-mnt-default px-4 py-3"
              >
                <div class="flex items-center justify-between mb-2">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-mono text-mnt-primary truncate">{{ pr.name }}</span>
                    <span
                      :class="[
                        'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0',
                        podStatusStyle(pr.status),
                      ]"
                    >
                      {{ pr.status }}
                    </span>
                  </div>
                  <span v-if="pr.node_name" class="text-xs text-mnt-muted font-mono">{{
                    pr.node_name
                  }}</span>
                </div>

                <!-- CPU -->
                <div class="mb-2">
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest"
                      >CPU</span
                    >
                    <span class="text-xs text-mnt-muted tabular-nums">{{
                      cpuDisplay(pr.cpu_millicores)
                    }}</span>
                  </div>
                </div>

                <!-- Memory bar -->
                <div>
                  <div class="flex items-center justify-between mb-1">
                    <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest"
                      >Memory</span
                    >
                    <span class="text-xs text-mnt-muted tabular-nums">
                      {{ formatBytes(pr.mem_bytes) }}
                      <template v-if="pr.mem_limit_bytes">
                        / {{ formatBytes(pr.mem_limit_bytes) }}</template
                      >
                      <span v-if="pr.mem_percent !== null" class="text-mnt-muted ml-1"
                        >({{ pr.mem_percent.toFixed(1) }}%)</span
                      >
                    </span>
                  </div>
                  <div
                    v-if="pr.mem_limit_bytes"
                    class="h-1.5 bg-mnt-primary border border-mnt-default rounded-full overflow-hidden"
                  >
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
      <span class="text-sm text-mnt-muted">Workload not found.</span>
    </div>
  </div>
</template>
