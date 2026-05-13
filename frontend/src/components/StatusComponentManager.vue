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
import { ref, computed, watch } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { useEdition } from '@/composables/useEdition'
import { useConfirm } from '@/composables/useConfirm'
import {
  createComponent,
  updateComponent,
  deleteComponent,
  type StatusComponent,
  type MonitorRef,
} from '@/services/statusApi'
import { listContainers } from '@/services/containerApi'
import { listEndpoints } from '@/services/endpointApi'
import { listHeartbeats } from '@/services/heartbeatApi'
import { listCertificates } from '@/services/certificateApi'

const store = useStatusAdminStore()
const { getQuota, reload } = useEdition()
const quota = getQuota('status_components')

// --- Monitor options ---

const allMonitorsByType = ref<Record<string, MonitorRef[]>>({})
const monitorOptionsLoading = ref(false)

async function loadAllMonitorOptions() {
  monitorOptionsLoading.value = true
  try {
    const [containers, endpoints, heartbeats, certs] = await Promise.all([
      listContainers(),
      listEndpoints(),
      listHeartbeats(),
      listCertificates(),
    ])
    allMonitorsByType.value = {
      container: containers.groups.flatMap(g => g.containers).map(c => ({ type: 'container', id: c.id, name: c.name })),
      endpoint: endpoints.endpoints.map(e => ({ type: 'endpoint', id: e.id, name: `${e.container_name} — ${e.target}` })),
      heartbeat: heartbeats.heartbeats.map(h => ({ type: 'heartbeat', id: h.id, name: h.name })),
      certificate: certs.certificates.map(c => ({ type: 'certificate', id: c.id, name: `${c.hostname}:${c.port}` })),
    }
    if (compositionMode.value === 'match-all') {
      matchAllCount.value = (allMonitorsByType.value[matchAllType.value] ?? []).length
    }
  } catch {
    allMonitorsByType.value = {}
  } finally {
    monitorOptionsLoading.value = false
  }
}

// --- Form state ---

const showCompForm = ref(false)
const editingCompId = ref<number | null>(null)
const createError = ref<string | null>(null)

// Composition mode — locked in edit mode
const compositionMode = ref<'explicit' | 'match-all'>('explicit')
const activeTypeTab = ref<string>('container')
const selectedMonitors = ref<MonitorRef[]>([])
const matchAllType = ref<string>('container')
const matchAllCount = ref<number | null>(null)
const searchQuery = ref('')

const isQuotaError = computed(() => {
  return createError.value?.includes('Upgrade to Pro') || false
})

const compForm = ref({
  display_name: '',
  visible: true,
  auto_incident: false,
})

// Monitor types
const monitorTypes = ['container', 'endpoint', 'heartbeat', 'certificate']
const monitorTypeLabels: Record<string, string> = {
  container: 'Container',
  endpoint: 'HTTP Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'SSL Certificate',
}

// Update match-all count when type changes
watch(matchAllType, (type) => {
  matchAllCount.value = (allMonitorsByType.value[type] ?? []).length
})

// Filtered monitors for the active tab
const filteredMonitors = computed(() => {
  const list = allMonitorsByType.value[activeTypeTab.value] ?? []
  if (!searchQuery.value) return list
  const q = searchQuery.value.toLowerCase()
  return list.filter(m => (m.name ?? '').toLowerCase().includes(q))
})

// Count of selected monitors per type
function selectedCountForType(type: string): number {
  return selectedMonitors.value.filter(m => m.type === type).length
}

function isMonitorSelected(m: MonitorRef): boolean {
  return selectedMonitors.value.some(s => s.type === m.type && s.id === m.id)
}

function toggleMonitor(m: MonitorRef) {
  const idx = selectedMonitors.value.findIndex(s => s.type === m.type && s.id === m.id)
  if (idx >= 0) {
    selectedMonitors.value.splice(idx, 1)
  } else {
    selectedMonitors.value.push({ type: m.type, id: m.id, name: m.name })
  }
}

function removeSelectedMonitor(m: MonitorRef) {
  const idx = selectedMonitors.value.findIndex(s => s.type === m.type && s.id === m.id)
  if (idx >= 0) selectedMonitors.value.splice(idx, 1)
}

const isFormValid = computed(() => {
  if (!compForm.value.display_name.trim()) return false
  if (compositionMode.value === 'explicit' && selectedMonitors.value.length === 0) return false
  return true
})

function resetCompForm() {
  compositionMode.value = 'explicit'
  activeTypeTab.value = 'container'
  selectedMonitors.value = []
  matchAllType.value = 'container'
  matchAllCount.value = null
  searchQuery.value = ''
  compForm.value = {
    display_name: '',
    visible: true,
    auto_incident: false,
  }
  editingCompId.value = null
  createError.value = null
  showCompForm.value = false
}

function startEditComp(c: StatusComponent) {
  editingCompId.value = c.id
  compositionMode.value = c.composition_mode
  matchAllType.value = c.match_all_type ?? 'container'
  selectedMonitors.value = (c.monitors ?? []).map(m => ({ type: m.type, id: m.id, name: m.name }))
  activeTypeTab.value = 'container'
  searchQuery.value = ''
  matchAllCount.value = null
  compForm.value = {
    display_name: c.display_name,
    visible: c.visible,
    auto_incident: c.auto_incident,
  }
  showCompForm.value = true
  loadAllMonitorOptions()
}

function startAddComp() {
  resetCompForm()
  showCompForm.value = true
  loadAllMonitorOptions()
}

async function submitCompForm() {
  createError.value = null
  try {
    if (editingCompId.value) {
      const updates: Parameters<typeof updateComponent>[1] = {
        display_name: compForm.value.display_name,
        visible: compForm.value.visible,
        auto_incident: compForm.value.auto_incident,
      }
      if (compositionMode.value === 'explicit') {
        updates.monitors = selectedMonitors.value.map(m => ({ type: m.type, id: m.id }))
      }
      await updateComponent(editingCompId.value, updates)
    } else {
      if (compositionMode.value === 'explicit') {
        await createComponent({
          composition_mode: 'explicit',
          monitors: selectedMonitors.value.map(m => ({ type: m.type, id: m.id })),
          display_name: compForm.value.display_name,
          visible: compForm.value.visible,
          auto_incident: compForm.value.auto_incident,
        })
      } else {
        await createComponent({
          composition_mode: 'match-all',
          match_all_type: matchAllType.value,
          display_name: compForm.value.display_name,
          visible: compForm.value.visible,
          auto_incident: compForm.value.auto_incident,
        })
      }
    }
    resetCompForm()
    store.fetchComponents()
    reload()
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to save component'
  }
}

const confirm = useConfirm()

async function handleDeleteComp(id: number) {
  const ok = await confirm({
    title: 'Remove component',
    message: 'Remove this component from the status page? This cannot be undone.',
    confirmLabel: 'Remove',
    destructive: true,
  })
  if (!ok) return
  await deleteComponent(id)
  store.fetchComponents()
  reload()
}

async function handleOverride(comp: StatusComponent, status: string) {
  await updateComponent(comp.id, { status_override: status })
  store.fetchComponents()
}

// --- Display helpers ---

const statusColors: Record<string, string> = {
  operational: 'var(--pb-status-ok)',
  degraded: 'var(--pb-status-warn)',
  partial_outage: 'var(--pb-status-critical)',
  major_outage: 'var(--pb-status-down)',
  under_maintenance: 'var(--pb-accent)',
}

const statusLabels: Record<string, string> = {
  operational: 'Operational',
  degraded: 'Degraded Performance',
  partial_outage: 'Partial Outage',
  major_outage: 'Major Outage',
  under_maintenance: 'Under Maintenance',
}

function formatStatus(s: string): string {
  return statusLabels[s] || s
}

const statusOverrideOptions: { value: string; label: string }[] = [
  { value: '', label: 'Auto (from monitor)' },
  { value: 'operational', label: 'Operational' },
  { value: 'degraded', label: 'Degraded Performance' },
  { value: 'partial_outage', label: 'Partial Outage' },
  { value: 'major_outage', label: 'Major Outage' },
  { value: 'under_maintenance', label: 'Under Maintenance' },
]

function componentSummary(c: StatusComponent): string {
  if (c.composition_mode === 'match-all') {
    return `All ${c.match_all_type ?? ''}s`
  }
  if (!c.monitors?.length) return 'No monitors'
  const counts: Record<string, number> = {}
  for (const m of c.monitors) {
    counts[m.type] = (counts[m.type] || 0) + 1
  }
  const typeLabels: Record<string, string> = {
    container: 'container',
    endpoint: 'endpoint',
    heartbeat: 'heartbeat',
    certificate: 'certificate',
  }
  return Object.entries(counts)
    .map(([t, n]) => `${n} ${typeLabels[t] || t}${n > 1 ? 's' : ''}`)
    .join(', ')
}
</script>

<template>
  <div>
    <!-- Components section -->
    <div>
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Status Components</h2>
        <div class="flex items-center gap-2">
          <span
            v-if="!quota.isUnlimited"
            class="rounded-full px-2.5 py-1 text-xs font-medium"
            :style="{
              backgroundColor: quota.isAtLimit ? 'var(--pb-status-down-bg)' : quota.nearLimit ? 'var(--pb-status-warn-bg)' : 'var(--pb-bg-elevated)',
              color: quota.isAtLimit ? 'var(--pb-status-down)' : quota.nearLimit ? 'var(--pb-status-warn)' : 'var(--pb-text-secondary)',
            }"
          >
            {{ quota.used }}/{{ quota.limit }}
          </span>
          <router-link
            v-if="quota.nearLimit && !quota.isAtLimit"
            :to="{ name: 'pro-edition' }"
            class="text-xs font-medium transition-opacity hover:opacity-80"
            style="color: var(--pb-accent)"
          >
            Upgrade
          </router-link>
          <button
            @click="startAddComp"
            :disabled="quota.isAtLimit"
            :title="quota.isAtLimit ? `Community edition limited to ${quota.limit} status components` : ''"
            class="rounded-md px-3 py-1.5 text-sm font-medium text-pb-primary transition-colors min-h-[44px]"
            :style="{
              background: 'var(--pb-accent)',
              opacity: quota.isAtLimit ? '0.5' : '1',
              cursor: quota.isAtLimit ? 'not-allowed' : 'pointer',
            }"
            @mouseenter="!quota.isAtLimit && (($event.target as HTMLElement).style.background = 'var(--pb-accent-hover)')"
            @mouseleave="($event.target as HTMLElement).style.background = 'var(--pb-accent)'"
          >
            Add Component
          </button>
        </div>
      </div>

      <!-- Form -->
      <div v-if="showCompForm" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">
          {{ editingCompId ? 'Edit Component' : 'New Component' }}
        </h3>

        <div
          v-if="createError"
          class="mb-3 rounded p-2 text-sm"
          :style="{
            backgroundColor: 'var(--pb-status-down-bg)',
            color: 'var(--pb-status-down)',
            borderRadius: 'var(--pb-radius-sm)',
          }"
        >
          <template v-if="isQuotaError">
            {{ createError.split('Upgrade to Pro')[0] }}
            <a
              href="/pro-edition"
              class="font-medium underline transition-opacity hover:opacity-80"
              style="color: #a78bfa"
            >
              Upgrade to Pro
            </a>
            {{ createError.split('Upgrade to Pro')[1] }}
          </template>
          <template v-else>
            {{ createError }}
          </template>
        </div>

        <form @submit.prevent="submitCompForm" class="space-y-4">

          <!-- Composition mode toggle — only shown when creating; locked in edit mode -->
          <div>
            <div class="mb-1 flex items-center gap-2">
              <span class="text-[10px] font-bold uppercase tracking-widest text-slate-500">Composition Mode</span>
              <span
                v-if="editingCompId"
                class="rounded px-1.5 py-0.5 text-[10px] text-slate-500"
                style="background: var(--pb-bg-elevated)"
                title="Mode is locked after creation; delete and recreate to change"
              >
                Locked
              </span>
            </div>
            <div class="flex gap-2">
              <button
                type="button"
                :disabled="!!editingCompId"
                @click="compositionMode = 'explicit'"
                class="flex-1 rounded-md border px-3 py-2 text-sm transition-colors"
                :style="{
                  background: compositionMode === 'explicit' ? 'rgba(59,130,246,0.12)' : 'var(--pb-bg-elevated)',
                  borderColor: compositionMode === 'explicit' ? '#3b82f6' : 'var(--pb-border-default)',
                  color: compositionMode === 'explicit' ? '#60a5fa' : 'var(--pb-text-secondary)',
                  opacity: editingCompId ? '0.5' : '1',
                  cursor: editingCompId ? 'not-allowed' : 'pointer',
                }"
              >
                Specific monitors
              </button>
              <button
                type="button"
                :disabled="!!editingCompId"
                @click="compositionMode = 'match-all'"
                class="flex-1 rounded-md border px-3 py-2 text-sm transition-colors"
                :style="{
                  background: compositionMode === 'match-all' ? 'rgba(139,92,246,0.12)' : 'var(--pb-bg-elevated)',
                  borderColor: compositionMode === 'match-all' ? '#8b5cf6' : 'var(--pb-border-default)',
                  color: compositionMode === 'match-all' ? '#a78bfa' : 'var(--pb-text-secondary)',
                  opacity: editingCompId ? '0.5' : '1',
                  cursor: editingCompId ? 'not-allowed' : 'pointer',
                }"
              >
                All monitors of one type
              </button>
            </div>
          </div>

          <!-- Explicit mode: per-type tabbed multi-select -->
          <div v-if="compositionMode === 'explicit'" class="space-y-3">
            <!-- Selected monitors chips -->
            <div v-if="selectedMonitors.length > 0" class="flex flex-wrap gap-1.5">
              <span
                v-for="m in selectedMonitors"
                :key="`${m.type}-${m.id}`"
                class="flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
                style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
              >
                <span class="text-[10px] uppercase tracking-wider" style="color: var(--pb-text-muted)">{{ m.type[0] }}</span>
                {{ m.name }}
                <button
                  type="button"
                  @click="removeSelectedMonitor(m)"
                  class="ml-0.5 opacity-60 hover:opacity-100"
                  style="color: var(--pb-text-muted)"
                >
                  ×
                </button>
              </span>
            </div>
            <p v-else class="text-xs text-slate-500">No monitors selected. Pick at least one below.</p>

            <!-- Type tabs -->
            <div class="flex gap-1 border-b" style="border-color: var(--pb-border-default)">
              <button
                v-for="type in monitorTypes"
                :key="type"
                type="button"
                @click="activeTypeTab = type; searchQuery = ''"
                class="relative px-3 py-1.5 text-xs transition-colors"
                :style="{
                  color: activeTypeTab === type ? 'var(--pb-text-primary)' : 'var(--pb-text-secondary)',
                  borderBottom: activeTypeTab === type ? '2px solid var(--pb-accent)' : '2px solid transparent',
                  marginBottom: '-1px',
                }"
              >
                {{ monitorTypeLabels[type] }}
                <span
                  v-if="selectedCountForType(type) > 0"
                  class="ml-1 rounded-full px-1.5 text-[10px] font-bold"
                  style="background: var(--pb-accent); color: var(--pb-text-primary)"
                >
                  {{ selectedCountForType(type) }}
                </span>
              </button>
            </div>

            <!-- Search -->
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Search..."
              class="w-full rounded-md border px-3 py-1.5 text-sm outline-none"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            />

            <!-- Monitor list -->
            <div
              class="max-h-48 overflow-y-auto rounded-md border"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default)"
            >
              <div v-if="monitorOptionsLoading" class="p-4 text-center text-xs text-slate-500">Loading...</div>
              <div v-else-if="filteredMonitors.length === 0" class="p-4 text-center text-xs text-slate-500">
                No {{ monitorTypeLabels[activeTypeTab] }}s found
              </div>
              <label
                v-for="m in filteredMonitors"
                :key="`${m.type}-${m.id}`"
                class="flex cursor-pointer items-center gap-3 border-b px-3 py-2 hover:bg-slate-800/25"
                style="border-color: var(--pb-border-default)"
              >
                <input
                  type="checkbox"
                  :checked="isMonitorSelected(m)"
                  @change="toggleMonitor(m)"
                  class="rounded"
                  style="accent-color: var(--pb-accent)"
                />
                <span class="text-sm" style="color: var(--pb-text-primary)">{{ m.name }}</span>
              </label>
            </div>
          </div>

          <!-- Match-all mode: single type dropdown + count preview -->
          <div v-else class="space-y-2">
            <div>
              <label class="block text-[10px] font-bold uppercase tracking-widest text-slate-500">Monitor Type</label>
              <select
                v-model="matchAllType"
                :disabled="!!editingCompId"
                class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm"
                style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
              >
                <option v-for="t in monitorTypes" :key="t" :value="t">{{ monitorTypeLabels[t] }}</option>
              </select>
            </div>
            <p v-if="monitorOptionsLoading" class="text-xs text-slate-500">Loading count...</p>
            <p v-else-if="matchAllCount !== null" class="text-xs text-slate-400">
              <span class="font-medium" style="color: var(--pb-text-primary)">{{ matchAllCount }}</span>
              monitor{{ matchAllCount !== 1 ? 's' : '' }} currently match
            </p>
          </div>

          <!-- Common fields -->
          <div>
            <label class="block text-[10px] font-bold uppercase tracking-widest text-slate-500">Display Name</label>
            <input
              v-model="compForm.display_name"
              required
              class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            />
          </div>

          <div class="flex items-center gap-4">
            <label class="flex items-center gap-2 text-sm" style="color: var(--pb-text-secondary)">
              <input v-model="compForm.visible" type="checkbox" class="rounded" style="accent-color: var(--pb-accent)" />
              Visible on public page
            </label>
            <label class="flex items-center gap-2 text-sm" style="color: var(--pb-text-secondary)">
              <input v-model="compForm.auto_incident" type="checkbox" class="rounded" style="accent-color: var(--pb-accent)" />
              Auto-create incidents
            </label>
          </div>

          <div class="flex gap-2">
            <button
              type="submit"
              :disabled="!isFormValid"
              class="rounded-md px-3 py-1.5 text-sm text-pb-primary"
              :style="{
                background: 'var(--pb-accent)',
                opacity: isFormValid ? '1' : '0.45',
                cursor: isFormValid ? 'pointer' : 'not-allowed',
              }"
            >
              Save
            </button>
            <button
              type="button"
              @click="resetCompForm"
              class="rounded-md border px-3 py-1.5 text-sm"
              style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
            >
              Cancel
            </button>
          </div>
        </form>
      </div>

      <!-- Empty state -->
      <div v-if="(store.components?.length ?? 0) === 0 && !store.componentsLoading" class="rounded-lg border p-6 text-center" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <p class="text-sm" style="color: var(--pb-text-muted)">No status components configured. Add components to appear on the public status page.</p>
      </div>

      <!-- Component list -->
      <div class="space-y-2">
        <div
          v-for="c in store.components"
          :key="c.id"
          class="rounded-lg border p-4"
          style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex items-start gap-3 min-w-0">
              <span class="mt-0.5 h-2.5 w-2.5 flex-shrink-0 rounded-full" :style="{ background: statusColors[c.effective_status] || 'var(--pb-text-muted)' }"></span>
              <div class="min-w-0">
                <!-- Name row -->
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ c.display_name }}</span>
                  <!-- Composition mode badge -->
                  <span
                    class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-widest"
                    :style="c.composition_mode === 'explicit'
                      ? 'background: rgba(59,130,246,0.15); color: #60a5fa'
                      : 'background: rgba(139,92,246,0.15); color: #a78bfa'"
                  >
                    {{ c.composition_mode === 'explicit' ? 'Explicit' : 'Match-all' }}
                  </span>
                  <span v-if="!c.visible" class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--pb-bg-elevated); color: var(--pb-text-muted)">hidden</span>
                  <span v-if="c.auto_incident" class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--pb-status-warn-bg); color: var(--pb-status-warn)">auto-incident</span>
                  <span v-if="c.status_override" class="rounded px-1.5 py-0.5 text-[10px]" style="background: rgba(139, 92, 246, 0.15); color: #a78bfa">overridden</span>
                </div>
                <!-- Summary + status -->
                <p class="mt-0.5 text-xs" style="color: var(--pb-text-muted)">
                  {{ componentSummary(c) }}
                  &middot; {{ formatStatus(c.effective_status) }}
                  <span v-if="c.status_override && c.derived_status !== c.effective_status"> (monitor: {{ formatStatus(c.derived_status) }})</span>
                </p>
              </div>
            </div>
            <div class="flex flex-shrink-0 items-center gap-2">
              <select
                @change="handleOverride(c, ($event.target as HTMLSelectElement).value)"
                class="rounded border px-2 py-1 text-xs"
                style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
              >
                <option v-for="s in statusOverrideOptions" :key="s.value" :value="s.value" :selected="(c.status_override || '') === s.value">
                  {{ s.label }}
                </option>
              </select>
              <button @click="startEditComp(c)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Edit</button>
              <button @click="handleDeleteComp(c.id)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-status-down); color: var(--pb-status-down)">Delete</button>
            </div>
          </div>

          <!-- Needs attention indicator -->
          <div v-if="c.needs_attention" class="mt-2 flex items-center gap-2 rounded border border-yellow-600/30 bg-yellow-900/20 px-3 py-2">
            <span class="text-xs text-yellow-400">No monitors assigned — hidden from public page</span>
            <button
              @click="startEditComp(c)"
              class="rounded px-2 py-0.5 text-xs font-medium"
              style="background: var(--pb-accent); color: var(--pb-text-primary)"
            >
              Fix
            </button>
            <button
              @click="handleDeleteComp(c.id)"
              class="rounded px-2 py-0.5 text-xs text-red-400"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
