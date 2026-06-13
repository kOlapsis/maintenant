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
import { fetchPodDetail, type K8sPodDetailResponse } from '@/services/kubernetesApi'
import { useResourcesStore } from '@/stores/resources'
import { timeAgo } from '@/utils/time'

const props = defineProps<{
  podNamespace: string
  podName: string
}>()

defineEmits<{
  close: []
}>()

type Tab = 'containers' | 'events'

const detail = ref<K8sPodDetailResponse | null>(null)
const loading = ref(true)
const activeTab = ref<Tab>('containers')

const tabs: { key: Tab; label: string }[] = [
  { key: 'containers', label: 'Containers' },
  { key: 'events', label: 'Events' },
]

onMounted(async () => {
  try {
    detail.value = await fetchPodDetail(
      props.podNamespace,
      props.podName,
      useResourcesStore().entityQuery,
    )
  } finally {
    loading.value = false
  }
})

function podStatusStyle(status: string): string {
  const s = status.toLowerCase()
  if (s === 'running') return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
  if (s === 'pending') return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
  if (s === 'succeeded') return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
  if (s === 'failed' || s === 'crashloopbackoff')
    return 'text-mnt-status-down bg-mnt-status-down border-mnt-sev-incident'
  return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
}

function containerStateStyle(state: string): string {
  const s = state.toLowerCase()
  if (s === 'running') return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
  if (s === 'waiting') return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
  if (s === 'terminated') return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
  return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
}

function eventTypeStyle(type: string): string {
  if (type === 'Warning') return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
  return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-16">
      <span class="text-sm text-mnt-muted">Loading pod…</span>
    </div>

    <template v-else-if="detail">
      <!-- Header -->
      <div class="px-5 pt-4 mnt-3 border-b border-mnt-default">
        <div class="min-w-0">
          <h2 class="text-base font-bold text-mnt-primary truncate font-mono">
            {{ detail.pod.name }}
          </h2>
          <div class="flex items-center gap-2 mt-1 flex-wrap">
            <span
              class="text-[10px] font-mono text-mnt-muted bg-mnt-elevated border border-mnt-default px-1.5 py-0.5 rounded"
            >
              {{ detail.pod.namespace }}
            </span>
            <span
              :class="[
                'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border',
                podStatusStyle(detail.pod.status),
              ]"
            >
              {{ detail.pod.status }}
            </span>
            <span v-if="detail.pod.status_reason" class="text-[10px] text-mnt-muted">
              {{ detail.pod.status_reason }}
            </span>
          </div>
        </div>

        <!-- Pod metadata grid -->
        <div class="mt-3 grid grid-cols-2 gap-x-6 gap-y-2 text-xs">
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">
              Node
            </p>
            <p class="text-mnt-secondary font-mono">{{ detail.pod.node_name || '—' }}</p>
          </div>
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">
              Pod IP
            </p>
            <p class="text-mnt-secondary font-mono">{{ detail.pod.pod_ip || '—' }}</p>
          </div>
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">
              Host IP
            </p>
            <p class="text-mnt-secondary font-mono">{{ detail.pod.host_ip || '—' }}</p>
          </div>
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">
              Restarts
            </p>
            <p
              :class="detail.pod.restart_count > 0 ? 'text-mnt-status-warn' : 'text-mnt-secondary'"
              class="font-semibold tabular-nums"
            >
              {{ detail.pod.restart_count }}
            </p>
          </div>
          <div v-if="detail.pod.workload_ref">
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">
              Workload
            </p>
            <p class="text-mnt-secondary font-mono truncate">{{ detail.pod.workload_ref }}</p>
          </div>
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-0.5">Age</p>
            <p class="text-mnt-secondary">{{ timeAgo(detail.pod.created_at) }}</p>
          </div>
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
          @click="activeTab = tab.key"
        >
          {{ tab.label }}
          <span v-if="tab.key === 'containers'" class="ml-1 text-[10px] text-mnt-muted">{{
            detail.pod.containers.length
          }}</span>
          <span v-if="tab.key === 'events'" class="ml-1 text-[10px] text-mnt-muted">{{
            detail.events.length
          }}</span>
        </button>
      </div>

      <!-- Tab content -->
      <div class="flex-1 overflow-y-auto px-5 py-4">
        <!-- Containers tab -->
        <template v-if="activeTab === 'containers'">
          <div
            v-if="detail.pod.containers.length === 0"
            class="text-sm text-mnt-muted py-4 text-center"
          >
            No container data
          </div>
          <div v-else class="space-y-2">
            <div
              v-for="container in detail.pod.containers"
              :key="container.name"
              class="bg-mnt-primary rounded-lg border border-mnt-default px-4 py-3"
            >
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-semibold text-mnt-primary">{{ container.name }}</span>
                <div class="flex items-center gap-2">
                  <span
                    v-if="!container.ready"
                    class="text-[10px] font-bold uppercase tracking-wider text-mnt-status-down bg-mnt-status-down border border-mnt-sev-incident px-1.5 py-0.5 rounded"
                    >not ready</span
                  >
                  <span
                    :class="[
                      'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border',
                      containerStateStyle(container.state),
                    ]"
                  >
                    {{ container.state }}
                  </span>
                </div>
              </div>
              <div class="mt-1.5 text-xs text-mnt-muted font-mono truncate">
                {{ container.image }}
              </div>
              <div class="flex items-center gap-4 mt-1 text-xs text-mnt-muted">
                <span v-if="container.state_reason" class="text-mnt-status-warn">{{
                  container.state_reason
                }}</span>
                <span v-if="container.restart_count > 0" class="text-mnt-status-warn"
                  >{{ container.restart_count }}↺</span
                >
                <span v-if="container.started_at">Started {{ timeAgo(container.started_at) }}</span>
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
      </div>
    </template>

    <!-- Error / no data -->
    <div v-else class="flex items-center justify-center py-16">
      <span class="text-sm text-mnt-muted">Pod not found.</span>
    </div>
  </div>
</template>
