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
import { ref } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { useConfirm } from '@/composables/useConfirm'
import {
  createIncident,
  postIncidentUpdate,
  deleteIncident,
  type Incident,
} from '@/services/statusApi'

const store = useStatusAdminStore()

const showCreateForm = ref(false)
const createForm = ref({
  title: '',
  severity: 'minor',
  component_ids: [] as number[],
  message: '',
})

const showUpdateForm = ref<number | null>(null)
const updateForm = ref({ status: 'identified', message: '' })

const statusFilter = ref('')

function resetCreateForm() {
  createForm.value = { title: '', severity: 'minor', component_ids: [], message: '' }
  showCreateForm.value = false
}

async function submitCreate() {
  await createIncident(createForm.value)
  resetCreateForm()
  store.fetchIncidents()
}

async function submitUpdate(incidentId: number) {
  await postIncidentUpdate(incidentId, updateForm.value)
  showUpdateForm.value = null
  updateForm.value = { status: 'identified', message: '' }
  store.fetchIncidents()
}

const confirm = useConfirm()

async function handleDelete(id: number) {
  const ok = await confirm({
    title: 'Delete incident',
    message: 'Remove this incident and all its updates? This cannot be undone.',
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  await deleteIncident(id)
  store.fetchIncidents()
}

function startPostUpdate(inc: Incident) {
  showUpdateForm.value = inc.id
  updateForm.value = { status: inc.status, message: '' }
}

function applyFilter() {
  store.fetchIncidents({ status: statusFilter.value || undefined })
}

const severityColors: Record<string, { bg: string; color: string }> = {
  minor: { bg: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)' },
  major: { bg: 'var(--pb-status-critical-bg)', color: 'var(--pb-status-critical)' },
  critical: { bg: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)' },
}

const statusBadgeColors: Record<string, { bg: string; color: string }> = {
  investigating: { bg: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)' },
  identified: { bg: 'var(--pb-status-critical-bg)', color: 'var(--pb-status-critical)' },
  monitoring: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--pb-accent)' },
  resolved: { bg: 'var(--pb-status-ok-bg)', color: 'var(--pb-status-ok)' },
}

const severityOptions = ['minor', 'major', 'critical']
const incidentStatusOptions = ['investigating', 'identified', 'monitoring', 'resolved']
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Incidents</h2>
      <div class="flex items-center gap-3">
        <select
          v-model="statusFilter"
          @change="applyFilter"
          class="rounded-md border px-2 py-1.5 text-sm"
          style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
        >
          <option value="">All statuses</option>
          <option v-for="s in incidentStatusOptions" :key="s" :value="s">{{ s }}</option>
        </select>
        <button
          @click="showCreateForm = true"
          class="rounded-md px-3 py-1.5 text-sm font-medium text-white"
          style="background: var(--pb-accent)"
        >
          Create Incident
        </button>
      </div>
    </div>

    <!-- Create form -->
    <div v-if="showCreateForm" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">New Incident</h3>
      <form @submit.prevent="submitCreate" class="space-y-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Title</label>
          <input v-model="createForm.title" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Severity</label>
          <select v-model="createForm.severity" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
            <option v-for="s in severityOptions" :key="s" :value="s">{{ s }}</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Affected Components</label>
          <div class="mt-1 max-h-32 space-y-1 overflow-y-auto rounded border p-2" style="border-color: var(--pb-border-default); background: var(--pb-bg-elevated)">
            <label v-for="c in store.components" :key="c.id" class="flex items-center gap-2 text-sm" style="color: var(--pb-text-secondary)">
              <input type="checkbox" :value="c.id" v-model="createForm.component_ids" class="rounded" style="accent-color: var(--pb-accent)" />
              {{ c.display_name }}
            </label>
            <p v-if="(store.components?.length ?? 0) === 0" class="text-xs" style="color: var(--pb-text-muted)">No components configured</p>
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Initial Message</label>
          <textarea v-model="createForm.message" required rows="2" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"></textarea>
        </div>
        <div class="flex gap-2">
          <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">Create</button>
          <button type="button" @click="resetCreateForm" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
        </div>
      </form>
    </div>

    <!-- Incident list -->
    <div v-if="(store.incidents?.length ?? 0) === 0 && !store.incidentsLoading" class="rounded-lg border p-6 text-center" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <p class="text-sm" style="color: var(--pb-text-muted)">No incidents</p>
    </div>

    <div class="space-y-3">
      <div
        v-for="inc in store.incidents"
        :key="inc.id"
        class="rounded-lg border p-4"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{
                background: (severityColors[inc.severity] || { bg: 'var(--pb-bg-elevated)' }).bg,
                color: (severityColors[inc.severity] || { color: 'var(--pb-text-secondary)' }).color,
              }"
            >
              {{ inc.severity }}
            </span>
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{
                background: (statusBadgeColors[inc.status] || { bg: 'var(--pb-bg-elevated)' }).bg,
                color: (statusBadgeColors[inc.status] || { color: 'var(--pb-text-secondary)' }).color,
              }"
            >
              {{ inc.status }}
            </span>
            <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ inc.title }}</span>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="inc.status !== 'resolved'"
              @click="startPostUpdate(inc)"
              class="rounded border px-2 py-1 text-xs"
              style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
            >
              Post Update
            </button>
            <button @click="handleDelete(inc.id)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-status-down); color: var(--pb-status-down)">Delete</button>
          </div>
        </div>

        <!-- Affected components -->
        <div v-if="inc.components?.length" class="mt-1 flex flex-wrap gap-1">
          <span
            v-for="c in inc.components"
            :key="c.component_id"
            class="rounded px-1.5 py-0.5 text-xs"
            style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
          >
            {{ c.name }}
          </span>
        </div>

        <!-- Post update form -->
        <div v-if="showUpdateForm === inc.id" class="mt-3 rounded border p-3" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default)">
          <form @submit.prevent="submitUpdate(inc.id)" class="space-y-2">
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Status</label>
              <select v-model="updateForm.status" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
                <option v-for="s in incidentStatusOptions" :key="s" :value="s">{{ s }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Message</label>
              <textarea v-model="updateForm.message" required rows="2" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default); color: var(--pb-text-primary)"></textarea>
            </div>
            <div class="flex gap-2">
              <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">Post Update</button>
              <button type="button" @click="showUpdateForm = null" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
            </div>
          </form>
        </div>

        <!-- Timeline -->
        <div v-if="inc.updates?.length" class="mt-3 border-t pt-2" style="border-color: var(--pb-border-subtle)">
          <div v-for="u in inc.updates" :key="u.id" class="flex gap-2 py-1">
            <span
              class="rounded px-1.5 py-0.5 text-xs"
              :style="{
                background: (statusBadgeColors[u.status] || { bg: 'var(--pb-bg-elevated)' }).bg,
                color: (statusBadgeColors[u.status] || { color: 'var(--pb-text-secondary)' }).color,
              }"
            >
              {{ u.status }}
            </span>
            <span class="flex-1 text-xs" style="color: var(--pb-text-secondary)">{{ u.message }}</span>
            <span class="text-xs" style="color: var(--pb-text-muted)">{{ new Date(u.created_at).toLocaleString() }}</span>
          </div>
        </div>

        <p class="mt-1 text-xs" style="color: var(--pb-text-muted)">Created {{ new Date(inc.created_at).toLocaleString() }}</p>
      </div>
    </div>

    <p v-if="store.incidentsTotal > (store.incidents?.length ?? 0)" class="mt-3 text-center text-xs" style="color: var(--pb-text-muted)">
      Showing {{ store.incidents?.length ?? 0 }} of {{ store.incidentsTotal }} incidents
    </p>
  </div>
</template>
