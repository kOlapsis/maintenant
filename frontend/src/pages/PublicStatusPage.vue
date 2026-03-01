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
interface StatusData {
  global_status: string
  global_message: string
  updated_at: string
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
  eventSource.addEventListener('status.updated', () => {
    fetchStatus()
  })
  eventSource.addEventListener('incident.created', () => fetchStatus())
  eventSource.addEventListener('incident.updated', () => fetchStatus())
  eventSource.addEventListener('component.status_changed', () => fetchStatus())
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
  if (s === 'under_maintenance') return { bg: 'bg-blue-500', text: 'Under Maintenance', icon: '⚙' }
  return { bg: 'bg-slate-600', text: data.value?.global_message || 'Loading…', icon: '·' }
})

const incidentSeverityStyle = (severity: string) => {
  if (severity === 'critical') return 'border-rose-500/40 bg-rose-500/5'
  if (severity === 'major') return 'border-rose-500/40 bg-rose-500/5'
  if (severity === 'minor') return 'border-amber-500/40 bg-amber-500/5'
  return 'border-blue-500/40 bg-blue-500/5'
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

function formatDate(iso: string) {
  return new Date(iso).toLocaleString('en-US', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
  })
}
</script>

<template>
  <div class="min-h-screen" style="background: #0f1115; color: #e2e8f0">
    <!-- Header -->
    <header class="border-b border-slate-800 bg-[#151923]">
      <div class="mx-auto max-w-3xl px-6 py-5 flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-7 h-7 bg-blue-600 rounded-md flex items-center justify-center shadow-lg shadow-blue-500/25">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="white">
              <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>
            </svg>
          </div>
          <span class="text-base font-bold text-white">PulseBoard</span>
        </div>
        <span class="text-xs text-slate-500 font-medium">Public Status Page</span>
      </div>
    </header>

    <!-- Organisation title -->
    <div v-if="organisationName" class="mx-auto max-w-3xl px-6 pt-10 pb-2 text-center">
      <h1 class="text-3xl font-black text-white tracking-tight">{{ organisationName }}</h1>
      <p class="text-sm text-slate-500 mt-1">Service Status</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex justify-center items-center py-24">
      <div class="h-6 w-6 animate-spin rounded-full border-2 border-slate-700 border-t-blue-500" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="mx-auto max-w-3xl px-6 py-16 text-center">
      <p class="text-rose-400 text-sm">{{ error }}</p>
    </div>

    <template v-else-if="data">
      <!-- Global banner -->
      <div :class="['py-10 text-white text-center', globalBanner.bg]">
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
                <span class="font-semibold text-slate-100 text-sm">{{ inc.title }}</span>
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
              class="rounded-xl border border-blue-500/30 bg-blue-500/5 p-5"
            >
              <div class="flex items-start justify-between gap-3 mb-1">
                <span class="font-semibold text-slate-100 text-sm">{{ maint.title }}</span>
                <span class="shrink-0 text-[10px] px-2 py-0.5 rounded bg-blue-500/15 text-blue-400 border border-blue-500/30 font-medium">
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
          <span>Powered by <span class="text-slate-500 font-semibold">PulseBoard</span></span>
        </footer>

      </div>
    </template>
  </div>
</template>
