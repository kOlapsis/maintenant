<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
import QuotaRefusal from '@/components/QuotaRefusal.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import TabNav, { type TabNavItem } from '@/components/ui/TabNav.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ChipToggle from '@/components/ui/ChipToggle.vue'

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
const editingCompId = ref<string | null>(null)
const createError = ref<unknown>(null)

// Composition mode — locked in edit mode
const compositionMode = ref<'explicit' | 'match-all'>('explicit')
const activeTypeTab = ref<string>('container')
const selectedMonitors = ref<MonitorRef[]>([])
const matchAllType = ref<string>('container')
const matchAllCount = ref<number | null>(null)
const searchQuery = ref('')


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

watch(activeTypeTab, () => {
  searchQuery.value = ''
})

const typeTabItems = computed<TabNavItem[]>(() =>
  monitorTypes.map((type) => {
    const count = selectedCountForType(type)
    return { value: type, label: monitorTypeLabels[type] ?? type, count: count > 0 ? count : undefined }
  }),
)

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
    createError.value = e
  }
}

const confirm = useConfirm()

async function handleDeleteComp(id: string) {
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
  operational: 'var(--mnt-status-ok)',
  degraded: 'var(--mnt-status-warn)',
  partial_outage: 'var(--mnt-status-critical)',
  major_outage: 'var(--mnt-status-down)',
  under_maintenance: 'var(--mnt-accent)',
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
        <h2 class="text-lg font-semibold" style="color: var(--mnt-text-primary)">Status Components</h2>
        <div class="flex items-center gap-2">
          <span
            v-if="!quota.isUnlimited"
            class="rounded-full px-2.5 py-1 text-xs font-medium"
            :style="{
              backgroundColor: quota.isAtLimit ? 'var(--mnt-status-down-bg)' : quota.nearLimit ? 'var(--mnt-status-warn-bg)' : 'var(--mnt-bg-elevated)',
              color: quota.isAtLimit ? 'var(--mnt-status-down)' : quota.nearLimit ? 'var(--mnt-status-warn)' : 'var(--mnt-text-secondary)',
            }"
          >
            {{ quota.used }}/{{ quota.limit }}
          </span>
          <router-link
            v-if="quota.nearLimit && !quota.isAtLimit"
            :to="{ name: 'editions' }"
            class="text-xs font-medium transition-opacity hover:opacity-80"
            style="color: var(--mnt-accent)"
          >
            Upgrade
          </router-link>
          <UiButton
            variant="primary"
            :disabled="quota.isAtLimit"
            :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} status components` : ''"
            @click="startAddComp"
          >
            Add Component
          </UiButton>
        </div>
      </div>

      <!-- Form -->
      <div v-if="showCompForm" class="mb-4 rounded-lg border p-4" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
        <h3 class="mb-3 text-sm font-medium" style="color: var(--mnt-text-primary)">
          {{ editingCompId ? 'Edit Component' : 'New Component' }}
        </h3>

        <QuotaRefusal v-if="createError" :error="createError" />

        <form @submit.prevent="submitCompForm" class="space-y-4">

          <!-- Composition mode toggle — only shown when creating; locked in edit mode -->
          <div>
            <div class="mb-1 flex items-center gap-2">
              <span class="text-[10px] font-bold uppercase tracking-widest text-mnt-muted">Composition Mode</span>
              <span
                v-if="editingCompId"
                class="rounded px-1.5 py-0.5 text-[10px] text-mnt-muted"
                style="background: var(--mnt-bg-elevated)"
                title="Mode is locked after creation; delete and recreate to change"
              >
                Locked
              </span>
            </div>
            <div :class="editingCompId ? 'pointer-events-none opacity-50' : ''">
              <SegmentedToggle
                v-model="compositionMode"
                ariaLabel="Composition mode"
                :options="[
                  { value: 'explicit', label: 'Specific monitors' },
                  { value: 'match-all', label: 'All monitors of one type' },
                ]"
              />
            </div>
          </div>

          <!-- Explicit mode: per-type tabbed multi-select -->
          <div v-if="compositionMode === 'explicit'" class="space-y-3">
            <!-- Selected monitors chips -->
            <div v-if="selectedMonitors.length > 0" class="flex flex-wrap gap-1.5">
              <ChipToggle
                v-for="m in selectedMonitors"
                :key="`${m.type}-${m.id}`"
                removable
                :remove-label="`Remove ${m.name}`"
                @remove="removeSelectedMonitor(m)"
              >
                <span class="text-[10px] uppercase tracking-wider text-mnt-muted">{{ m.type[0] }}</span>
                {{ m.name }}
              </ChipToggle>
            </div>
            <p v-else class="text-xs text-mnt-muted">No monitors selected. Pick at least one below.</p>

            <!-- Type tabs -->
            <TabNav v-model="activeTypeTab" :items="typeTabItems" ariaLabel="Monitor type" />

            <!-- Search -->
            <SearchInput v-model="searchQuery" placeholder="Search..." />

            <!-- Monitor list -->
            <div
              class="max-h-48 overflow-y-auto rounded-md border"
              style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-default)"
            >
              <div v-if="monitorOptionsLoading" class="p-4 text-center text-xs text-mnt-muted">Loading...</div>
              <div v-else-if="filteredMonitors.length === 0" class="p-4 text-center text-xs text-mnt-muted">
                No {{ monitorTypeLabels[activeTypeTab] }}s found
              </div>
              <div
                v-for="m in filteredMonitors"
                :key="`${m.type}-${m.id}`"
                class="border-b px-3 py-2 hover:bg-mnt-elevated"
                style="border-color: var(--mnt-border-default)"
              >
                <CheckboxInput :model-value="isMonitorSelected(m)" :label="m.name" @update:model-value="() => toggleMonitor(m)" />
              </div>
            </div>
          </div>

          <!-- Match-all mode: single type dropdown + count preview -->
          <div v-else class="space-y-2">
            <FormField label="Monitor Type">
              <template #default="{ id, describedBy, invalid }">
                <SelectInput
                  :id="id"
                  v-model="matchAllType"
                  :disabled="!!editingCompId"
                  :aria-describedby="describedBy"
                  :invalid="invalid"
                  :options="monitorTypes.map((t) => ({ value: t, label: monitorTypeLabels[t] ?? t }))"
                />
              </template>
            </FormField>
            <p v-if="monitorOptionsLoading" class="text-xs text-mnt-muted">Loading count...</p>
            <p v-else-if="matchAllCount !== null" class="text-xs text-mnt-muted">
              <span class="font-medium" style="color: var(--mnt-text-primary)">{{ matchAllCount }}</span>
              monitor{{ matchAllCount !== 1 ? 's' : '' }} currently match
            </p>
          </div>

          <!-- Common fields -->
          <FormField label="Display Name" required>
            <template #default="{ id, describedBy, invalid }">
              <TextInput :id="id" v-model="compForm.display_name" required :aria-describedby="describedBy" :invalid="invalid" />
            </template>
          </FormField>

          <div class="flex items-center gap-4">
            <CheckboxInput v-model="compForm.visible" label="Visible on public page" />
            <CheckboxInput v-model="compForm.auto_incident" label="Auto-create incidents" />
          </div>

          <div class="flex gap-2">
            <UiButton type="submit" variant="primary" :disabled="!isFormValid">Save</UiButton>
            <UiButton type="button" variant="secondary" @click="resetCompForm">Cancel</UiButton>
          </div>
        </form>
      </div>

      <!-- Empty state -->
      <div v-if="(store.components?.length ?? 0) === 0 && !store.componentsLoading" class="rounded-lg border p-6 text-center" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
        <p class="text-sm" style="color: var(--mnt-text-muted)">No status components configured. Add components to appear on the public status page.</p>
      </div>

      <!-- Component list -->
      <div class="space-y-2">
        <div
          v-for="c in store.components"
          :key="c.id"
          class="rounded-lg border p-4"
          style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex items-start gap-3 min-w-0">
              <span class="mt-0.5 h-2.5 w-2.5 flex-shrink-0 rounded-full" :style="{ background: statusColors[c.effective_status] || 'var(--mnt-text-muted)' }"></span>
              <div class="min-w-0">
                <!-- Name row -->
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ c.display_name }}</span>
                  <!-- Composition mode badge -->
                  <span
                    class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-widest"
                    :style="c.composition_mode === 'explicit'
                      ? 'background: rgba(59,130,246,0.15); color: #60a5fa'
                      : 'background: rgba(139,92,246,0.15); color: var(--mnt-accent)'"
                  >
                    {{ c.composition_mode === 'explicit' ? 'Explicit' : 'Match-all' }}
                  </span>
                  <span v-if="!c.visible" class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-muted)">hidden</span>
                  <span v-if="c.auto_incident" class="rounded px-1.5 py-0.5 text-[10px]" style="background: var(--mnt-status-warn-bg); color: var(--mnt-status-warn)">auto-incident</span>
                  <span v-if="c.status_override" class="rounded px-1.5 py-0.5 text-[10px]" style="background: rgba(139, 92, 246, 0.15); color: var(--mnt-accent)">overridden</span>
                </div>
                <!-- Summary + status -->
                <p class="mt-0.5 text-xs" style="color: var(--mnt-text-muted)">
                  {{ componentSummary(c) }}
                  &middot; {{ formatStatus(c.effective_status) }}
                  <span v-if="c.status_override && c.derived_status !== c.effective_status"> (monitor: {{ formatStatus(c.derived_status) }})</span>
                </p>
              </div>
            </div>
            <div class="flex flex-shrink-0 items-center gap-2">
              <SelectInput
                :model-value="c.status_override || ''"
                size="sm"
                :options="statusOverrideOptions"
                @change="handleOverride(c, ($event.target as HTMLSelectElement).value)"
              />
              <UiButton variant="secondary" size="sm" @click="startEditComp(c)">Edit</UiButton>
              <UiButton variant="danger-ghost" size="sm" @click="handleDeleteComp(c.id)">Delete</UiButton>
            </div>
          </div>

          <!-- Needs attention indicator -->
          <div v-if="c.needs_attention" class="mt-2 flex items-center gap-2 rounded border border-mnt-sev-warning bg-mnt-status-warn px-3 py-2">
            <span class="text-xs text-mnt-status-warn">No monitors assigned — hidden from public page</span>
            <UiButton variant="primary" size="sm" @click="startEditComp(c)">Fix</UiButton>
            <UiButton variant="ghost" size="sm" @click="handleDeleteComp(c.id)">Delete</UiButton>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
