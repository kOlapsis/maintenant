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
    if (filterStatus.value && !pod.status.toLowerCase().includes(filterStatus.value.toLowerCase())) return false
    return true
  })
})

function podStatusStyle(status: string): string {
  const s = status.toLowerCase()
  if (s === 'running') return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
  if (s === 'pending') return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
  if (s === 'succeeded') return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
  if (s === 'failed' || s === 'crashloopbackoff') return 'text-red-400 bg-red-400/10 border-red-400/20'
  return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
}

function restartCountStyle(count: number): string {
  if (count === 0) return 'text-slate-500'
  if (count < 5) return 'text-amber-400'
  return 'text-red-400'
}
</script>

<template>
  <div class="bg-[#12151C] rounded-xl border border-slate-800 overflow-hidden">
    <!-- Filter bar -->
    <div class="flex flex-wrap items-center gap-2 px-4 py-3 border-b border-slate-800">
      <input
        v-model="filterNamespace"
        type="text"
        placeholder="Namespace…"
        class="bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-300 placeholder-slate-600 focus:outline-none focus:border-slate-700 w-32"
      />
      <input
        v-model="filterWorkload"
        type="text"
        placeholder="Workload…"
        class="bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-300 placeholder-slate-600 focus:outline-none focus:border-slate-700 w-32"
      />
      <input
        v-model="filterNode"
        type="text"
        placeholder="Node…"
        class="bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-300 placeholder-slate-600 focus:outline-none focus:border-slate-700 w-28"
      />
      <input
        v-model="filterStatus"
        type="text"
        placeholder="Status…"
        class="bg-[#0B0E13] border border-slate-800 rounded-lg px-3 py-1.5 text-xs text-slate-300 placeholder-slate-600 focus:outline-none focus:border-slate-700 w-24"
      />
      <span class="ml-auto text-xs text-slate-500 tabular-nums">
        {{ filteredPods.length }}/{{ pods.length }}
      </span>
    </div>

    <!-- Empty state -->
    <div v-if="filteredPods.length === 0" class="px-6 py-10 text-center">
      <p class="text-sm text-slate-500">No pods found</p>
    </div>

    <!-- Pod rows -->
    <div v-else class="divide-y divide-slate-800/60">
      <div
        v-for="pod in filteredPods"
        :key="`${pod.namespace}/${pod.name}`"
        class="px-4 py-3 hover:bg-slate-800/25 transition-all cursor-pointer group"
        @click="emit('select', pod)"
      >
        <div class="flex items-center justify-between gap-4">
          <!-- Left: name + namespace + status -->
          <div class="flex items-center gap-2 min-w-0">
            <div class="min-w-0">
              <span class="text-sm text-white font-medium truncate group-hover:text-pb-green-400 transition-colors block">
                {{ pod.name }}
              </span>
              <span class="text-xs text-slate-500 font-mono">{{ pod.namespace }}</span>
            </div>
          </div>

          <!-- Right: status + restarts + node + IP + age -->
          <div class="flex items-center gap-3 flex-shrink-0">
            <div class="flex items-center gap-1">
              <span :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', podStatusStyle(pod.status)]">
                {{ pod.status }}
              </span>
              <span
                v-if="pod.status_reason"
                class="text-[10px] text-slate-500 hidden sm:block truncate max-w-20"
              >
                {{ pod.status_reason }}
              </span>
            </div>
            <span
              :class="['text-xs font-semibold tabular-nums hidden sm:block', restartCountStyle(pod.restart_count)]"
              :title="`${pod.restart_count} restarts`"
            >
              {{ pod.restart_count }}↺
            </span>
            <span class="text-xs text-slate-500 font-mono hidden md:block truncate max-w-28">
              {{ pod.node_name || '—' }}
            </span>
            <span class="text-xs text-slate-600 font-mono hidden lg:block">
              {{ pod.pod_ip || '—' }}
            </span>
            <span class="text-xs text-slate-500 tabular-nums hidden md:block">
              {{ timeAgo(pod.created_at) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
