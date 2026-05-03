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
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import StatusComponentBreakdown from '@/components/StatusComponentBreakdown.vue'
import { useStatusPageI18n } from '@/composables/useStatusPageI18n'
import type { MonitorRef } from '@/services/statusApi'

// --- Personalization settings ---
interface PublicSettings {
  version: number
  title: string
  subtitle: string
  colors: {
    bg: string; surface: string; border: string; text: string; accent: string
    status_operational: string; status_degraded: string; status_partial: string; status_major: string
  }
  assets: {
    logo?: { data_url: string; alt_text: string }
    favicon?: { data_url: string; alt_text: string }
    hero?: { data_url: string; alt_text: string }
  }
  announcement: { enabled: boolean; html: string; url: string }
  footer: {
    html: string
    links: { label: string; url: string }[]
    powered_by: { label: string; url: string }
  }
  locale: string
  timezone: string
  date_format: string
}

const settings = ref<PublicSettings | null>(null)
const locale = computed(() => settings.value?.locale ?? 'en')
const { t } = useStatusPageI18n(locale)

async function fetchSettings() {
  try {
    const res = await fetch('/status/settings.json')
    if (res.ok) settings.value = await res.json()
  } catch {
    // graceful degradation — defaults already applied via null checks
  }
}

// Apply CSS custom properties from the palette
watch(settings, (s) => {
  if (!s) return
  const root = document.documentElement
  root.style.setProperty('--mnt-bg', s.colors.bg)
  root.style.setProperty('--mnt-surface', s.colors.surface)
  root.style.setProperty('--mnt-border', s.colors.border)
  root.style.setProperty('--mnt-text', s.colors.text)
  root.style.setProperty('--mnt-accent', s.colors.accent)
  root.style.setProperty('--mnt-status-operational', s.colors.status_operational)
  root.style.setProperty('--mnt-status-degraded', s.colors.status_degraded)
  root.style.setProperty('--mnt-status-partial', s.colors.status_partial)
  root.style.setProperty('--mnt-status-major', s.colors.status_major)
  // Update tab title
  if (s.title) document.title = s.title
  // Update favicon
  if (s.assets?.favicon?.data_url) {
    const link: HTMLLinkElement = document.querySelector("link[rel*='icon']") ?? document.createElement('link')
    link.type = 'image/x-icon'
    link.rel = 'shortcut icon'
    link.href = s.assets.favicon.data_url
    document.head.appendChild(link)
  }
}, { immediate: false })

// --- Status data ---
interface IncidentUpdate { status: string; message: string; created_at: string }
interface IncidentBrief {
  id: number; title: string; severity: string; status: string
  components: string[]; created_at: string; latest_update?: IncidentUpdate
}
interface MaintenanceBrief {
  id: number; title: string; starts_at: string; ends_at: string; components: string[]
}
interface ComponentBrief { id: number; name: string; status: string; monitors?: MonitorRef[] }
interface StatusData {
  global_status: string; global_message: string; updated_at: string
  components: ComponentBrief[]; active_incidents: IncidentBrief[]; upcoming_maintenance: MaintenanceBrief[]
}

const data = ref<StatusData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const expandedComponents = ref<Set<number>>(new Set())

function toggleExpanded(id: number) {
  if (expandedComponents.value.has(id)) expandedComponents.value.delete(id)
  else expandedComponents.value.add(id)
}

function handleRowKeydown(e: KeyboardEvent, id: number) {
  if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleExpanded(id) }
  else if (e.key === 'Escape') { expandedComponents.value.delete(id); (e.currentTarget as HTMLElement).blur() }
}

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

function handleComponentChangedEvent(e: Event) {
  const msgEvent = e as MessageEvent
  if (msgEvent.data) {
    try {
      const payload = JSON.parse(msgEvent.data) as { id?: number; monitors?: MonitorRef[]; status?: string; name?: string }
      if (payload.id !== undefined && data.value) {
        const comp = data.value.components.find(c => c.id === payload.id)
        if (comp) {
          if (payload.status !== undefined) comp.status = payload.status
          if (payload.name !== undefined) comp.name = payload.name
          if (payload.monitors !== undefined) comp.monitors = payload.monitors
          return
        }
      }
    } catch { /* fall through to full refresh */ }
  }
  fetchStatus()
}

function connectSSE() {
  eventSource = new EventSource('/status/events')
  eventSource.addEventListener('status.component_changed', handleComponentChangedEvent)
  eventSource.addEventListener('status.global_changed', () => fetchStatus())
  eventSource.addEventListener('status.incident_created', () => fetchStatus())
  eventSource.addEventListener('status.incident_updated', () => fetchStatus())
  eventSource.addEventListener('status.incident_resolved', () => fetchStatus())
  eventSource.addEventListener('status.maintenance_started', () => fetchStatus())
  eventSource.addEventListener('status.maintenance_ended', () => fetchStatus())
}

onMounted(() => {
  fetchSettings()
  fetchStatus()
  connectSSE()
})

onUnmounted(() => { eventSource?.close() })

const globalBanner = computed(() => {
  const s = data.value?.global_status
  if (s === 'operational') return { color: 'var(--mnt-status-operational, #22C55E)', text: t('globalAllOperational'), icon: '✓' }
  if (s === 'degraded') return { color: 'var(--mnt-status-degraded, #EAB308)', text: t('globalDegraded'), icon: '⚠' }
  if (s === 'partial_outage') return { color: 'var(--mnt-status-partial, #F97316)', text: t('globalPartialOutage'), icon: '⚠' }
  if (s === 'major_outage') return { color: 'var(--mnt-status-major, #EF4444)', text: t('globalMajorOutage'), icon: '✕' }
  return { color: 'var(--mnt-accent, #22C55E)', text: data.value?.global_message || 'Loading…', icon: '·' }
})

const incidentSeverityStyle = (severity: string) => {
  if (severity === 'critical' || severity === 'major') return 'border-rose-500/40 bg-rose-500/5'
  if (severity === 'minor') return 'border-amber-500/40 bg-amber-500/5'
  return 'border-pb-green-500/40 bg-pb-green-500/5'
}

const incidentStatusLabel = (status: string) => {
  const map: Record<string, string> = {
    investigating: t('incidentStatusInvestigating'),
    identified: t('incidentStatusIdentified'),
    monitoring: t('incidentStatusMonitoring'),
    resolved: t('incidentStatusResolved'),
  }
  return map[status] || status
}

const componentStatusStyle = (status: string) => {
  const styles: Record<string, { dot: string; label: string; text: string }> = {
    operational: { dot: 'bg-emerald-500', label: t('statusOperational'), text: 'text-pb-status-ok' },
    degraded: { dot: 'bg-amber-500', label: t('statusDegraded'), text: 'text-amber-400' },
    partial_outage: { dot: 'bg-amber-500', label: t('statusPartialOutage'), text: 'text-amber-400' },
    major_outage: { dot: 'bg-rose-500', label: t('statusMajorOutage'), text: 'text-pb-status-down' },
    under_maintenance: { dot: 'bg-pb-green-500', label: 'Under Maintenance', text: 'text-pb-green-400' },
  }
  return styles[status] || { dot: 'bg-slate-500', label: status, text: 'text-slate-400' }
}

function formatDate(iso: string) {
  const tz = settings.value?.timezone || undefined
  const loc = settings.value?.locale || 'en'
  if (settings.value?.date_format === 'absolute') {
    return new Intl.DateTimeFormat(loc, {
      day: 'numeric', month: 'long', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
      timeZone: tz,
    }).format(new Date(iso))
  }
  const diff = (Date.now() - new Date(iso).getTime()) / 1000
  const rtf = new Intl.RelativeTimeFormat(loc, { numeric: 'auto' })
  if (Math.abs(diff) < 60) return rtf.format(-Math.round(diff), 'second')
  if (Math.abs(diff) < 3600) return rtf.format(-Math.round(diff / 60), 'minute')
  if (Math.abs(diff) < 86400) return rtf.format(-Math.round(diff / 3600), 'hour')
  return rtf.format(-Math.round(diff / 86400), 'day')
}
</script>

<template>
  <div
    class="min-h-screen text-white"
    :style="{
      backgroundColor: 'var(--mnt-bg, #0B0E13)',
      color: 'var(--mnt-text, #FFFFFF)',
    }"
  >
    <!-- Announcement banner -->
    <div
      v-if="settings?.announcement?.enabled"
      :style="{ backgroundColor: 'var(--mnt-accent, #22C55E)' }"
      class="py-3 px-6 text-center text-sm font-medium"
    >
      <a
        v-if="settings.announcement.url"
        :href="settings.announcement.url"
        target="_blank"
        rel="noopener noreferrer"
        class="block"
        v-html="settings.announcement.html"
      />
      <div v-else v-html="settings.announcement.html" />
    </div>

    <!-- Header -->
    <header :style="{ backgroundColor: 'var(--mnt-surface, #12151C)', borderColor: 'var(--mnt-border, #1F2937)' }" class="border-b">
      <div class="mx-auto max-w-3xl px-6 py-5 flex items-center justify-between">
        <img
          v-if="settings?.assets?.logo"
          :src="settings.assets.logo.data_url"
          :alt="settings.assets.logo.alt_text || 'Logo'"
          class="h-10 object-contain"
        />
        <img v-else src="/logo.svg" alt="maintenant" />
        <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text, #FFFFFF)', opacity: 0.5 }">Status</span>
      </div>
    </header>

    <!-- Hero -->
    <div v-if="settings?.assets?.hero" class="mx-auto max-w-3xl px-0 overflow-hidden">
      <img
        :src="settings.assets.hero.data_url"
        :alt="settings.assets.hero.alt_text || 'Hero'"
        class="w-full object-cover max-h-48"
      />
    </div>

    <!-- Title / subtitle -->
    <div class="mx-auto max-w-3xl px-6 pt-10 pb-2 text-center">
      <h1 class="text-3xl font-black tracking-tight">{{ settings?.title || 'System Status' }}</h1>
      <p v-if="settings?.subtitle" class="text-sm mt-1 opacity-60">{{ settings.subtitle }}</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex justify-center items-center py-24">
      <div class="h-6 w-6 animate-spin rounded-full border-2 border-slate-700" :style="{ borderTopColor: 'var(--mnt-accent, #22C55E)' }" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="mx-auto max-w-3xl px-6 py-16 text-center">
      <p class="text-sm text-red-400">{{ error }}</p>
    </div>

    <template v-else-if="data">
      <!-- Global banner -->
      <div
        class="py-10 text-center"
        :style="{ backgroundColor: globalBanner.color }"
      >
        <div class="mx-auto max-w-3xl px-6">
          <div class="text-3xl font-black tracking-tight mb-1 text-white">
            {{ globalBanner.icon }} {{ globalBanner.text }}
          </div>
          <p v-if="data.global_message" class="text-sm opacity-80 mt-1 text-white">{{ data.global_message }}</p>
          <p class="text-xs opacity-60 mt-2 text-white">{{ t('updatedAt') }} {{ formatDate(data.updated_at) }}</p>
        </div>
      </div>

      <div class="mx-auto max-w-3xl px-6 py-10 space-y-10">
        <!-- Components -->
        <section v-if="data.components?.length">
          <div
            class="rounded-xl border divide-y"
            :style="{
              backgroundColor: 'var(--mnt-surface, #12151C)',
              borderColor: 'var(--mnt-border, #1F2937)',
            }"
          >
            <div v-for="comp in data.components" :key="comp.id">
              <button
                type="button"
                class="flex w-full items-center justify-between px-5 py-3.5 text-left transition-colors hover:bg-slate-800/20 focus:outline-none focus-visible:ring-2 focus-visible:ring-slate-500"
                :aria-expanded="expandedComponents.has(comp.id)"
                :aria-controls="`breakdown-${comp.id}`"
                @click="toggleExpanded(comp.id)"
                @keydown="handleRowKeydown($event, comp.id)"
              >
                <span class="text-sm font-medium">{{ comp.name }}</span>
                <div class="flex items-center gap-2">
                  <span :class="['text-xs font-medium', componentStatusStyle(comp.status).text]">
                    {{ componentStatusStyle(comp.status).label }}
                  </span>
                  <span :class="['h-2 w-2 rounded-full', componentStatusStyle(comp.status).dot]" />
                  <span
                    v-if="comp.monitors?.length"
                    class="text-slate-600 transition-transform"
                    :style="{ transform: expandedComponents.has(comp.id) ? 'rotate(180deg)' : 'rotate(0deg)' }"
                  >&#8964;</span>
                </div>
              </button>
              <div v-if="expandedComponents.has(comp.id) && comp.monitors?.length" :id="`breakdown-${comp.id}`" class="px-5 pb-3">
                <StatusComponentBreakdown :monitors="comp.monitors" />
              </div>
            </div>
          </div>
        </section>

        <div v-else-if="!data.active_incidents?.length && !data.upcoming_maintenance?.length" class="text-center py-6">
          <p class="text-sm opacity-50">No status components configured.</p>
        </div>

        <!-- Active Incidents -->
        <section v-if="data.active_incidents?.length">
          <h2 class="text-xs font-bold uppercase tracking-widest mb-3 opacity-50">{{ t('sectionActiveIncidents') }}</h2>
          <div class="space-y-3">
            <div
              v-for="inc in data.active_incidents"
              :key="inc.id"
              :class="['rounded-xl border p-5', incidentSeverityStyle(inc.severity)]"
            >
              <div class="flex items-start justify-between gap-3 mb-2">
                <span class="font-semibold text-sm">{{ inc.title }}</span>
                <span class="shrink-0 text-xs px-2 py-0.5 rounded bg-slate-800 text-slate-400 border border-slate-700">
                  {{ incidentStatusLabel(inc.status) }}
                </span>
              </div>
              <div v-if="inc.latest_update" class="text-sm text-slate-400 mb-2">{{ inc.latest_update.message }}</div>
              <div class="flex flex-wrap gap-1.5">
                <span v-for="comp in inc.components" :key="comp" class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800/80 text-slate-400 font-medium border border-slate-700/50">{{ comp }}</span>
              </div>
              <p class="text-[10px] text-slate-600 mt-2">{{ formatDate(inc.created_at) }}</p>
            </div>
          </div>
        </section>

        <!-- Upcoming Maintenance -->
        <section v-if="data.upcoming_maintenance?.length">
          <h2 class="text-xs font-bold uppercase tracking-widest mb-3 opacity-50">{{ t('sectionUpcomingMaintenance') }}</h2>
          <div class="space-y-3">
            <div
              v-for="maint in data.upcoming_maintenance"
              :key="maint.id"
              class="rounded-xl border border-pb-green-500/30 bg-pb-green-500/5 p-5"
            >
              <div class="flex items-start justify-between gap-3 mb-1">
                <span class="font-semibold text-sm">{{ maint.title }}</span>
                <span class="shrink-0 text-[10px] px-2 py-0.5 rounded bg-pb-green-500/15 text-pb-green-400 border border-pb-green-500/30 font-medium">{{ t('maintenanceScheduled') }}</span>
              </div>
              <p class="text-xs text-slate-500 mb-2">{{ formatDate(maint.starts_at) }} {{ t('maintenanceTo') }} {{ formatDate(maint.ends_at) }}</p>
              <div class="flex flex-wrap gap-1.5">
                <span v-for="comp in maint.components" :key="comp" class="text-[10px] px-1.5 py-0.5 rounded bg-slate-800/80 text-slate-400 font-medium border border-slate-700/50">{{ comp }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- FAQ -->
        <section v-if="settings?.footer?.links && settings.footer.links.length > 0 || false">
        </section>

        <!-- Footer -->
        <footer class="pt-6 border-t space-y-4" :style="{ borderColor: 'var(--mnt-border, #1F2937)' }">
          <!-- Footer HTML text -->
          <div v-if="settings?.footer?.html" class="text-xs text-slate-400" v-html="settings.footer.html" />

          <!-- Footer links -->
          <div v-if="settings?.footer?.links?.length" class="flex flex-wrap gap-4">
            <a
              v-for="link in settings.footer.links"
              :key="link.url"
              :href="link.url"
              target="_blank"
              rel="noopener noreferrer"
              class="text-xs hover:opacity-80 transition-opacity"
              :style="{ color: 'var(--mnt-accent, #22C55E)' }"
            >{{ link.label }}</a>
          </div>

          <!-- Powered by (always rendered) -->
          <div class="flex items-center justify-between text-xs text-slate-600">
            <span />
            <a
              :href="settings?.footer?.powered_by?.url || 'https://maintenant.dev'"
              target="_blank"
              rel="noopener noreferrer"
              class="hover:text-slate-400 transition-colors"
            >
              {{ settings?.footer?.powered_by?.label || 'Powered by Maintenant' }}
            </a>
          </div>
        </footer>
      </div>
    </template>
  </div>
</template>
