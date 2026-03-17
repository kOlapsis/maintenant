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
import { ref, watch } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { useConfirm } from '@/composables/useConfirm'
import {
  createComponent,
  updateComponent,
  deleteComponent,
  type StatusComponent,
} from '@/services/statusApi'
import { listContainers, type Container } from '@/services/containerApi'
import { listEndpoints, type Endpoint } from '@/services/endpointApi'
import { listHeartbeats, type Heartbeat } from '@/services/heartbeatApi'
import { listCertificates, type CertMonitor } from '@/services/certificateApi'

const store = useStatusAdminStore()

// --- Monitor options ---
interface MonitorOption {
  id: number
  label: string
}

const monitorOptions = ref<MonitorOption[]>([])
const monitorOptionsLoading = ref(false)

async function loadMonitorOptions(type: string) {
  monitorOptionsLoading.value = true
  monitorOptions.value = []
  try {
    switch (type) {
      case 'container': {
        const res = await listContainers()
        const all = res.groups.flatMap(g => g.containers)
        monitorOptions.value = all.map((c: Container) => ({ id: c.id, label: c.name }))
        break
      }
      case 'endpoint': {
        const res = await listEndpoints()
        monitorOptions.value = res.endpoints.map((e: Endpoint) => ({ id: e.id, label: `${e.container_name} — ${e.target}` }))
        break
      }
      case 'heartbeat': {
        const res = await listHeartbeats()
        monitorOptions.value = res.heartbeats.map((h: Heartbeat) => ({ id: h.id, label: h.name }))
        break
      }
      case 'certificate': {
        const res = await listCertificates()
        monitorOptions.value = res.certificates.map((c: CertMonitor) => ({ id: c.id, label: `${c.hostname}:${c.port}` }))
        break
      }
    }
  } catch {
    monitorOptions.value = []
  } finally {
    monitorOptionsLoading.value = false
  }
}

// --- Components ---
const showCompForm = ref(false)
const editingCompId = ref<number | null>(null)
const compForm = ref({
  monitor_type: 'container',
  monitor_id: 0,
  display_name: '',
  group_id: null as number | null,
  visible: true,
  auto_incident: false,
})

watch(() => compForm.value.monitor_type, (type) => {
  if (!editingCompId.value) {
    compForm.value.monitor_id = 0
    compForm.value.display_name = ''
    loadMonitorOptions(type)
  }
})

function onMonitorSelected(id: number) {
  compForm.value.monitor_id = id
  if (id === 0) {
    if (!compForm.value.display_name) {
      compForm.value.display_name = `All ${monitorTypeLabels[compForm.value.monitor_type]}s`
    }
    return
  }
  const opt = monitorOptions.value.find(o => o.id === id)
  if (opt && !compForm.value.display_name) {
    compForm.value.display_name = opt.label
  }
}

function resetCompForm() {
  compForm.value = {
    monitor_type: 'container',
    monitor_id: 0,
    display_name: '',
    group_id: null,
    visible: true,
    auto_incident: false,
  }
  editingCompId.value = null
  showCompForm.value = false
}

function startEditComp(c: StatusComponent) {
  editingCompId.value = c.id
  compForm.value = {
    monitor_type: c.monitor_type,
    monitor_id: c.monitor_id,
    display_name: c.display_name,
    group_id: c.group_id,
    visible: c.visible,
    auto_incident: c.auto_incident,
  }
  showCompForm.value = true
  loadMonitorOptions(c.monitor_type)
}

function startAddComp() {
  resetCompForm()
  showCompForm.value = true
  loadMonitorOptions(compForm.value.monitor_type)
}

async function submitCompForm() {
  if (editingCompId.value) {
    await updateComponent(editingCompId.value, {
      display_name: compForm.value.display_name,
      group_id: compForm.value.group_id,
      visible: compForm.value.visible,
      auto_incident: compForm.value.auto_incident,
    })
  } else {
    await createComponent(compForm.value)
  }
  resetCompForm()
  store.fetchComponents()
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
}

async function handleOverride(comp: StatusComponent, status: string) {
  // Send empty string to clear the override (backend converts "" to NULL)
  await updateComponent(comp.id, { status_override: status })
  store.fetchComponents()
}

const statusColors: Record<string, string> = {
  operational: 'var(--pb-status-ok)',
  degraded: 'var(--pb-status-warn)',
  partial_outage: 'var(--pb-status-critical)',
  major_outage: 'var(--pb-status-down)',
  under_maintenance: 'var(--pb-accent)',
}

const monitorTypes = ['container', 'endpoint', 'heartbeat', 'certificate']
const monitorTypeLabels: Record<string, string> = {
  container: 'Container',
  endpoint: 'HTTP Endpoint',
  heartbeat: 'Heartbeat',
  certificate: 'SSL Certificate',
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
</script>

<template>
  <div>
    <!-- Components section -->
    <div>
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Status Components</h2>
        <button
          @click="startAddComp"
          class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
          style="background: var(--pb-accent)"
          @mouseenter="($event.target as HTMLElement).style.background = 'var(--pb-accent-hover)'"
          @mouseleave="($event.target as HTMLElement).style.background = 'var(--pb-accent)'"
        >
          Add Component
        </button>
      </div>

      <div v-if="showCompForm" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">
          {{ editingCompId ? 'Edit Component' : 'New Component' }}
        </h3>
        <form @submit.prevent="submitCompForm" class="space-y-3">
          <div v-if="!editingCompId" class="space-y-3">
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Monitor Type</label>
              <select v-model="compForm.monitor_type" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
                <option v-for="t in monitorTypes" :key="t" :value="t">{{ monitorTypeLabels[t] }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">{{ monitorTypeLabels[compForm.monitor_type] }}</label>
              <select
                :value="compForm.monitor_id"
                @change="onMonitorSelected(Number(($event.target as HTMLSelectElement).value))"
                :disabled="monitorOptionsLoading"
                class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm"
                style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
              >
                <option :value="0">{{ monitorOptionsLoading ? 'Loading...' : 'All (globalized)' }}</option>
                <option v-for="opt in monitorOptions" :key="opt.id" :value="opt.id">{{ opt.label }}</option>
              </select>
            </div>
          </div>
          <div>
            <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Display Name</label>
            <input v-model="compForm.display_name" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
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
            <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">Save</button>
            <button type="button" @click="resetCompForm" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
          </div>
        </form>
      </div>

      <div v-if="(store.components?.length ?? 0) === 0 && !store.componentsLoading" class="rounded-lg border p-6 text-center" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <p class="text-sm" style="color: var(--pb-text-muted)">No status components configured. Add components to appear on the public status page.</p>
      </div>

      <div class="space-y-2">
        <div
          v-for="c in store.components"
          :key="c.id"
          class="rounded-lg border p-4"
          style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <span class="h-2.5 w-2.5 rounded-full" :style="{ background: statusColors[c.effective_status] || 'var(--pb-text-muted)' }"></span>
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ c.display_name }}</span>
                  <span v-if="!c.visible" class="rounded px-1.5 py-0.5 text-xs" style="background: var(--pb-bg-elevated); color: var(--pb-text-muted)">hidden</span>
                  <span v-if="c.auto_incident" class="rounded px-1.5 py-0.5 text-xs" style="background: var(--pb-status-warn-bg); color: var(--pb-status-warn)">auto-incident</span>
                  <span v-if="c.status_override" class="rounded px-1.5 py-0.5 text-xs" style="background: rgba(139, 92, 246, 0.15); color: #a78bfa">overridden</span>
                </div>
                <p class="text-xs" style="color: var(--pb-text-muted)">
                  {{ monitorTypeLabels[c.monitor_type] || c.monitor_type }}
                  &middot; {{ formatStatus(c.effective_status) }}
                  <span v-if="c.status_override && c.derived_status !== c.effective_status"> (monitor: {{ formatStatus(c.derived_status) }})</span>
                </p>
              </div>
            </div>
            <div class="flex items-center gap-2">
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
        </div>
      </div>
    </div>
  </div>
</template>
