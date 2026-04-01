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
import { ref, computed, onMounted, inject } from 'vue'
import { fetchSwarmServices, type SwarmServiceResponse } from '@/services/swarmApi'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { timeAgo } from '@/utils/time'
import { Layers, ChevronDown, ChevronRight } from 'lucide-vue-next'

interface ServiceGroup {
  stack: string
  services: SwarmServiceResponse[]
}

const { openDetail } = inject(detailSlideOverKey)!

const services = ref<SwarmServiceResponse[]>([])
const loading = ref(true)
const expandedGroups = ref<Set<string>>(new Set())

const groups = computed<ServiceGroup[]>(() => {
  const map = new Map<string, SwarmServiceResponse[]>()
  for (const svc of services.value) {
    const stack = svc.stack_name || 'Standalone'
    if (!map.has(stack)) map.set(stack, [])
    map.get(stack)!.push(svc)
  }
  return Array.from(map.entries())
    .map(([stack, svcs]) => ({
      stack,
      services: svcs.sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .sort((a, b) => {
      // Put "Standalone" last
      if (a.stack === 'Standalone') return 1
      if (b.stack === 'Standalone') return -1
      return a.stack.localeCompare(b.stack)
    })
})

onMounted(async () => {
  try {
    const resp = await fetchSwarmServices()
    services.value = resp.services
    // Expand all groups by default
    for (const g of groups.value) {
      expandedGroups.value.add(g.stack)
    }
  } finally {
    loading.value = false
  }
})

function toggleGroup(stack: string) {
  if (expandedGroups.value.has(stack)) {
    expandedGroups.value.delete(stack)
  } else {
    expandedGroups.value.add(stack)
  }
}

function replicaColor(running: number, desired: number): string {
  if (running >= desired) return 'text-emerald-400'
  if (running > 0) return 'text-amber-400'
  return 'text-red-400'
}

function imageTag(image: string): { name: string; tag: string } {
  const colonIdx = image.lastIndexOf(':')
  if (colonIdx < 0) return { name: image, tag: 'latest' }
  return { name: image.slice(0, colonIdx), tag: image.slice(colonIdx + 1) }
}

function updateStateStyle(state: string): string {
  switch (state) {
    case 'updating': return 'text-sky-400 bg-sky-400/10 border-sky-400/20'
    case 'paused': return 'text-amber-400 bg-amber-400/10 border-amber-400/20'
    case 'rollback_started':
    case 'rollback_paused':
    case 'rollback_completed':
      return 'text-red-400 bg-red-400/10 border-red-400/20'
    default: return 'text-slate-400 bg-slate-400/10 border-slate-400/20'
  }
}

function handleSelect(svc: SwarmServiceResponse) {
  openDetail('swarm-service', svc.service_id)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
      <!-- Page header -->
      <div class="mb-6">
        <h1 class="text-2xl font-black text-pb-primary">Services</h1>
        <p class="mt-1 text-sm text-slate-500">Swarm services grouped by stack</p>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-16">
        <span class="text-sm text-slate-500">Loading services…</span>
      </div>

      <!-- Empty -->
      <div
        v-else-if="services.length === 0"
        class="bg-pb-surface rounded-xl border border-slate-800 px-6 py-12 text-center"
      >
        <Layers :size="32" class="mx-auto mb-3 text-slate-600" />
        <p class="text-sm text-slate-500">No services found</p>
        <p class="mt-1 text-xs text-slate-600">Make sure this node is a Swarm manager</p>
      </div>

      <!-- Groups -->
      <div v-else class="space-y-4">
        <div
          v-for="group in groups"
          :key="group.stack"
          class="bg-pb-surface rounded-xl border border-slate-800 overflow-hidden"
        >
          <!-- Group header -->
          <button
            class="w-full flex items-center justify-between px-4 py-3 hover:bg-slate-800/25 transition-all"
            @click="toggleGroup(group.stack)"
          >
            <div class="flex items-center gap-3">
              <component
                :is="expandedGroups.has(group.stack) ? ChevronDown : ChevronRight"
                :size="14"
                class="text-slate-500 flex-shrink-0"
              />
              <span class="text-sm font-semibold text-pb-primary">{{ group.stack }}</span>
              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded">
                {{ group.services.length }} service{{ group.services.length === 1 ? '' : 's' }}
              </span>
            </div>
            <div class="flex items-center gap-2 text-xs text-slate-500">
              <span
                :class="[
                  'font-semibold tabular-nums',
                  group.services.every(s => s.running_replicas >= s.desired_replicas)
                    ? 'text-emerald-400'
                    : group.services.some(s => s.running_replicas > 0)
                      ? 'text-amber-400'
                      : 'text-red-400',
                ]"
              >
                {{ group.services.reduce((sum, s) => sum + s.running_replicas, 0) }}/{{ group.services.reduce((sum, s) => sum + s.desired_replicas, 0) }}
              </span>
            </div>
          </button>

          <!-- Service rows -->
          <div v-if="expandedGroups.has(group.stack)" class="border-t border-slate-800 divide-y divide-slate-800/60">
            <div
              v-for="svc in group.services"
              :key="svc.service_id"
              class="px-4 py-3 hover:bg-slate-800/25 transition-all cursor-pointer group"
              @click="handleSelect(svc)"
            >
              <div class="flex items-center justify-between gap-4">
                <!-- Left: name + badges -->
                <div class="flex items-center gap-2 min-w-0">
                  <span class="text-sm text-pb-primary font-medium truncate group-hover:text-pb-green-400 transition-colors">
                    {{ svc.name }}
                  </span>
                  <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500 bg-slate-400/10 border border-slate-400/20 px-1.5 py-0.5 rounded flex-shrink-0">
                    {{ svc.mode }}
                  </span>
                  <span
                    v-if="svc.update_status?.state && svc.update_status.state !== 'completed'"
                    :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0', updateStateStyle(svc.update_status.state)]"
                  >
                    {{ svc.update_status.state.replace(/_/g, ' ') }}
                  </span>
                </div>

                <!-- Right: replicas + image + updated -->
                <div class="flex items-center gap-4 flex-shrink-0">
                  <span :class="['text-sm font-semibold tabular-nums', replicaColor(svc.running_replicas, svc.desired_replicas)]">
                    {{ svc.running_replicas }}/{{ svc.desired_replicas }}
                  </span>
                  <div class="hidden sm:flex items-center gap-1.5 max-w-48">
                    <span class="text-xs text-slate-500 font-mono truncate">{{ imageTag(svc.image).name.split('/').pop() }}</span>
                    <span class="text-[10px] font-mono text-slate-600 bg-slate-800 px-1 py-0.5 rounded flex-shrink-0">
                      {{ imageTag(svc.image).tag }}
                    </span>
                  </div>
                  <span class="text-xs text-slate-500 tabular-nums hidden md:block">{{ timeAgo(svc.created_at) }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
