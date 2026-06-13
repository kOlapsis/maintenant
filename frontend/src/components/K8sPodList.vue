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
import { ref, computed } from 'vue'
import { type K8sPod } from '@/services/kubernetesApi'
import { timeAgo } from '@/utils/time'
import HostBadge from '@/components/HostBadge.vue'

const props = defineProps<{
  pods: K8sPod[]
}>()

const emit = defineEmits<{
  select: [pod: K8sPod]
}>()

const filterNamespace = ref('')
const filterWorkload = ref('')
const filterNode = ref('')
const filterStatus = ref('')

const filteredPods = computed(() => {
  return props.pods.filter((pod) => {
    if (filterNamespace.value && !pod.namespace.includes(filterNamespace.value)) return false
    if (filterWorkload.value && !pod.workload_ref.includes(filterWorkload.value)) return false
    if (filterNode.value && !pod.node_name.includes(filterNode.value)) return false
    if (filterStatus.value && !pod.status.toLowerCase().includes(filterStatus.value.toLowerCase()))
      return false
    return true
  })
})

function podStatusStyle(status: string): string {
  const s = status.toLowerCase()
  if (s === 'running') return 'text-pb-status-ok bg-pb-status-ok border-emerald-400/20'
  if (s === 'pending') return 'text-pb-status-warn bg-pb-status-warn border-pb-sev-warning'
  if (s === 'succeeded') return 'text-pb-secondary bg-pb-elevated border-pb-default'
  if (s === 'failed' || s === 'crashloopbackoff')
    return 'text-pb-status-down bg-pb-status-down border-pb-sev-incident'
  return 'text-pb-muted bg-pb-elevated border-pb-default'
}

function restartCountStyle(count: number): string {
  if (count === 0) return 'text-pb-muted'
  if (count < 5) return 'text-pb-status-warn'
  return 'text-pb-status-down'
}
</script>

<template>
  <div class="bg-pb-surface rounded-xl border border-pb-default overflow-hidden">
    <!-- Filter bar -->
    <div class="flex flex-wrap items-center gap-2 px-4 py-3 border-b border-pb-default">
      <input
        v-model="filterNamespace"
        type="text"
        placeholder="Namespace…"
        class="bg-pb-primary border border-pb-default rounded-lg px-3 py-1.5 text-xs text-pb-secondary placeholder:text-pb-muted focus:outline-none focus:border-pb-default w-32"
      />
      <input
        v-model="filterWorkload"
        type="text"
        placeholder="Workload…"
        class="bg-pb-primary border border-pb-default rounded-lg px-3 py-1.5 text-xs text-pb-secondary placeholder:text-pb-muted focus:outline-none focus:border-pb-default w-32"
      />
      <input
        v-model="filterNode"
        type="text"
        placeholder="Node…"
        class="bg-pb-primary border border-pb-default rounded-lg px-3 py-1.5 text-xs text-pb-secondary placeholder:text-pb-muted focus:outline-none focus:border-pb-default w-28"
      />
      <input
        v-model="filterStatus"
        type="text"
        placeholder="Status…"
        class="bg-pb-primary border border-pb-default rounded-lg px-3 py-1.5 text-xs text-pb-secondary placeholder:text-pb-muted focus:outline-none focus:border-pb-default w-24"
      />
      <span class="ml-auto text-xs text-pb-muted tabular-nums">
        {{ filteredPods.length }}/{{ pods.length }}
      </span>
    </div>

    <!-- Empty state -->
    <div v-if="filteredPods.length === 0" class="px-6 py-10 text-center">
      <p class="text-sm text-pb-muted">No pods found</p>
    </div>

    <!-- Pod rows -->
    <div v-else class="divide-y divide-slate-800/60">
      <div
        v-for="pod in filteredPods"
        :key="`${pod.namespace}/${pod.name}`"
        class="px-4 py-3 hover:bg-pb-elevated transition-all cursor-pointer group"
        @click="emit('select', pod)"
      >
        <div class="flex items-center justify-between gap-4">
          <!-- Left: name + namespace + status -->
          <div class="flex items-center gap-2 min-w-0">
            <div class="min-w-0">
              <span
                class="text-sm text-pb-primary font-medium truncate group-hover:text-pb-green-400 transition-colors block"
              >
                {{ pod.name }}
              </span>
              <div class="flex items-center gap-2">
                <span class="text-xs text-pb-muted font-mono">{{ pod.namespace }}</span>
                <HostBadge
                  :agent-id="pod.agent_id"
                  :hostname="pod.agent_hostname"
                  :label="pod.agent_label"
                />
              </div>
            </div>
          </div>

          <!-- Right: status + restarts + node + IP + age -->
          <div class="flex items-center gap-3 flex-shrink-0">
            <div class="flex items-center gap-1">
              <span
                :class="[
                  'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border',
                  podStatusStyle(pod.status),
                ]"
              >
                {{ pod.status }}
              </span>
              <span
                v-if="pod.status_reason"
                class="text-[10px] text-pb-muted hidden sm:block truncate max-w-20"
              >
                {{ pod.status_reason }}
              </span>
            </div>
            <span
              :class="[
                'text-xs font-semibold tabular-nums hidden sm:block',
                restartCountStyle(pod.restart_count),
              ]"
              :title="`${pod.restart_count} restarts`"
            >
              {{ pod.restart_count }}↺
            </span>
            <span class="text-xs text-pb-muted font-mono hidden md:block truncate max-w-28">
              {{ pod.node_name || '—' }}
            </span>
            <span class="text-xs text-pb-muted font-mono hidden lg:block">
              {{ pod.pod_ip || '—' }}
            </span>
            <span class="text-xs text-pb-muted tabular-nums hidden md:block">
              {{ timeAgo(pod.created_at) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
