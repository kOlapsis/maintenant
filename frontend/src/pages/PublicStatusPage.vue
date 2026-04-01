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
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useEdition } from '@/composables/useEdition'

const API_BASE = import.meta.env.VITE_API_BASE || '/api/v1'
const { organisationName } = useEdition()

interface IncidentUpdate {
  status: string
  message: string
  created_at: string
}
interface IncidentBrief {
  id: number
  title: string
  severity: string
  status: string
  components: string[]
  created_at: string
  latest_update?: IncidentUpdate
}
interface MaintenanceBrief {
  id: number
  title: string
  starts_at: string
  ends_at: string
  components: string[]
}
interface ComponentBrief {
  id: number
  name: string
  status: string
}
interface GroupBrief {
  name: string
  components: ComponentBrief[]
}
interface StatusData {
  global_status: string
  global_message: string
  updated_at: string
  groups: GroupBrief[]
  active_incidents: IncidentBrief[]
  upcoming_maintenance: MaintenanceBrief[]
}

const data = ref<StatusData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

let eventSource: EventSource | null = null

async function fetchStatus() {
  try {
    const res = await fetch('/status/api')
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    data.value = await res.json()
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load status'
  } finally {
    loading.value = false
  }
}

function connectSSE() {
  eventSource = new EventSource('/status/events')
  eventSource.addEventListener('status.component_changed', () => fetchStatus())
  eventSource.addEventListener('status.global_changed', () => fetchStatus())
  eventSource.addEventListener('status.incident_created', () => fetchStatus())
  eventSource.addEventListener('status.incident_updated', () => fetchStatus())
  eventSource.addEventListener('status.incident_resolved', () => fetchStatus())
  eventSource.addEventListener('status.maintenance_started', () => fetchStatus())
  eventSource.addEventListener('status.maintenance_ended', () => fetchStatus())
}

onMounted(() => {
  fetchStatus()
  connectSSE()
})

onUnmounted(() => {
  eventSource?.close()
})

const globalBanner = computed(() => {
  const s = data.value?.global_status
  if (s === 'operational') return { bg: 'bg-emerald-500', text: 'All Systems Operational', icon: '✓' }
  if (s === 'degraded') return { bg: 'bg-amber-500', text: 'Degraded Performance', icon: '⚠' }
  if (s === 'partial_outage') return { bg: 'bg-amber-500', text: 'Partial Outage', icon: '⚠' }
  if (s === 'major_outage') return { bg: 'bg-rose-500', text: 'Major Outage', icon: '✕' }
  if (s === 'under_maintenance') return { bg: 'bg-pb-green-500', text: 'Under Maintenance', icon: '⚙' }
  return { bg: 'bg-slate-600', text: data.value?.global_message || 'Loading…', icon: '·' }
})

const incidentSeverityStyle = (severity: string) => {
  if (severity === 'critical') return 'border-rose-500/40 bg-rose-500/5'
  if (severity === 'major') return 'border-rose-500/40 bg-rose-500/5'
  if (severity === 'minor') return 'border-amber-500/40 bg-amber-500/5'
  return 'border-pb-green-500/40 bg-pb-green-500/5'
}

const incidentStatusLabel = (status: string) => {
  const map: Record<string, string> = {
    investigating: 'Investigating',
    identified: 'Identified',
    monitoring: 'Monitoring',
    resolved: 'Resolved',
  }
  return map[status] || status
}

const componentStatusStyle = (status: string) => {
  const styles: Record<string, { dot: string; label: string; text: string }> = {
    operational: { dot: 'bg-emerald-500', label: 'Operational', text: 'text-emerald-400' },
    degraded: { dot: 'bg-amber-500', label: 'Degraded Performance', text: 'text-amber-400' },
    partial_outage: { dot: 'bg-amber-500', label: 'Partial Outage', text: 'text-amber-400' },
    major_outage: { dot: 'bg-rose-500', label: 'Major Outage', text: 'text-rose-400' },
    under_maintenance: { dot: 'bg-pb-green-500', label: 'Under Maintenance', text: 'text-pb-green-400' },
  }
  return styles[status] || { dot: 'bg-slate-500', label: status, text: 'text-slate-400' }
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString('en-US', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
  })
}
</script>

<template>
  <div class="min-h-screen bg-pb-primary text-pb-primary">
    <!-- Header -->
    <header class="border-b border-slate-800 bg-pb-surface">
      <div class="mx-auto max-w-3xl px-6 py-5 flex items-center justify-between">
        <img src="/logo.svg" alt="maintenant"/>
        <span class="text-xs text-slate-500 font-medium">Public Status Page</span>
      </div>
    </header>

    <!-- Organisation title -->
    <div v-if="organisationName" class="mx-auto max-w-3xl px-6 pt-10 pb-2 text-center">
      <h1 class="text-3xl font-black text-pb-primary tracking-tight">{{ organisationName }}</h1>
      <p class="text-sm text-slate-500 mt-1">Service Status</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex justify-center items-center py-24">
      <div class="h-6 w-6 animate-spin rounded-full border-2 border-slate-700 border-t-pb-green-500" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="mx-auto max-w-3xl px-6 py-16 text-center">
      <p class="text-rose-400 text-sm">{{ error }}</p>
    </div>

    <template v-else-if="data">
      <!-- Global banner -->
      <div :class="['py-10 text-pb-primary text-center', globalBanner.bg]">
        <div class="mx-auto max-w-3xl px-6">
          <div class="text-3xl font-black tracking-tight mb-1">
            {{ globalBanner.icon }} {{ globalBanner.text }}
          </div>
          <p v-if="data.global_message" class="text-sm opacity-80 mt-1">{{ data.global_message }}</p>
          <p class="text-xs opacity-60 mt-2">
            Updated {{ formatDate(data.updated_at) }}
          </p>
        </div>
      </div>

      <div class="mx-auto max-w-3xl px-6 py-10 space-y-10">

        <!-- Component Groups -->
        <section v-if="data.groups?.length">
          <div class="space-y-6">
            <div v-for="group in data.groups" :key="group.name">
              <h2
                v-if="data.groups.length > 1 && group.name !== 'Other'"
                class="text-xs font-bold text-slate-500 uppercase tracking-widest mb-3"
              >{{ group.name }}</h2>
              <div class="rounded-xl border border-slate-800 bg-pb-surface divide-y divide-slate-800">
                <div
                  v-for="comp in group.components"
                  :key="comp.id"
                  class="flex items-center justify-between px-5 py-3.5"
                >
                  <span class="text-sm font-medium text-pb-primary">{{ comp.name }}</span>
                  <div class="flex items-center gap-2">
                    <span :class="['text-xs font-medium', componentStatusStyle(comp.status).text]">
                      {{ componentStatusStyle(comp.status).label }}
                    </span>
                    <span :class="['h-2 w-2 rounded-full', componentStatusStyle(comp.status).dot]" />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <div v-else-if="!data.active_incidents?.length && !data.upcoming_maintenance?.length" class="text-center py-6">
          <p class="text-sm text-slate-500">No status components configured.</p>
        </div>

        <!-- Active Incidents -->
        <section v-if="data.active_incidents?.length">
          <h2 class="text-xs font-bold text-slate-500 uppercase tracking-widest mb-3">Active Incidents</h2>
          <div class="space-y-3">
            <div
              v-for="inc in data.active_incidents"
              :key="inc.id"
              :class="['rounded-xl border p-5', incidentSeverityStyle(inc.severity)]"
            >
              <div class="flex items-start justify-between gap-3 mb-2">
                <span class="font-semibold text-pb-primary text-sm">{{ inc.title }}</span>
                <span class="shrink-0 text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-400 border border-slate-700">
                  {{ incidentStatusLabel(inc.status) }}
                </span>
              </div>
              <div v-if="inc.latest_update" class="text-sm text-slate-400 mb-2">
                {{ inc.latest_update.message }}
              </div>
              <div class="flex flex-wrap gap-1.5">
                <span
                  v-for="comp in inc.components"
                  :key="comp"
                  class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800/80 text-slate-400 font-medium border border-slate-700/50"
                >
                  {{ comp }}
                </span>
              </div>
              <p class="text-[10px] text-slate-600 mt-2">{{ formatDate(inc.created_at) }}</p>
            </div>
          </div>
        </section>

        <!-- Upcoming Maintenance -->
        <section v-if="data.upcoming_maintenance?.length">
          <h2 class="text-xs font-bold text-slate-500 uppercase tracking-widest mb-3">Scheduled Maintenance</h2>
          <div class="space-y-3">
            <div
              v-for="maint in data.upcoming_maintenance"
              :key="maint.id"
              class="rounded-xl border border-pb-green-500/30 bg-pb-green-500/5 p-5"
            >
              <div class="flex items-start justify-between gap-3 mb-1">
                <span class="font-semibold text-pb-primary text-sm">{{ maint.title }}</span>
                <span class="shrink-0 text-[10px] px-2 py-0.5 rounded bg-pb-green-500/15 text-pb-green-400 border border-pb-green-500/30 font-medium">
                  Scheduled
                </span>
              </div>
              <p class="text-xs text-slate-500 mb-2">
                {{ formatDate(maint.starts_at) }} → {{ formatDate(maint.ends_at) }}
              </p>
              <div class="flex flex-wrap gap-1.5">
                <span
                  v-for="comp in maint.components"
                  :key="comp"
                  class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800/80 text-slate-400 font-medium border border-slate-700/50"
                >
                  {{ comp }}
                </span>
              </div>
            </div>
          </div>
        </section>

        <!-- Footer -->
        <footer class="pt-6 border-t border-slate-800 flex items-center justify-between text-xs text-slate-600">
          <span>Powered by <span class="text-slate-500 font-semibold">maintenant</span></span>
        </footer>

      </div>
    </template>
  </div>
</template>
