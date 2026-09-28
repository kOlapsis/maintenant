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
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const store = useStatusAdminStore()

const showCreateForm = ref(false)
const createForm = ref({
  title: '',
  severity: 'minor',
  component_ids: [] as string[],
  message: '',
})

const showUpdateForm = ref<string | null>(null)
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

async function submitUpdate(incidentId: string) {
  await postIncidentUpdate(incidentId, updateForm.value)
  showUpdateForm.value = null
  updateForm.value = { status: 'identified', message: '' }
  store.fetchIncidents()
}

const confirm = useConfirm()

async function handleDelete(id: string) {
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
  minor: { bg: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)' },
  major: { bg: 'var(--mnt-status-critical-bg)', color: 'var(--mnt-status-critical)' },
  critical: { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' },
}

const statusBadgeColors: Record<string, { bg: string; color: string }> = {
  investigating: { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' },
  identified: { bg: 'var(--mnt-status-critical-bg)', color: 'var(--mnt-status-critical)' },
  monitoring: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--mnt-accent)' },
  resolved: { bg: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok)' },
}

const severityOptions = ['minor', 'major', 'critical']
const incidentStatusOptions = ['investigating', 'identified', 'monitoring', 'resolved']
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--mnt-text-primary)">Incidents</h2>
      <div class="flex items-center gap-3">
        <SelectInput
          v-model="statusFilter"
          size="sm"
          :options="[{ value: '', label: 'All statuses' }, ...incidentStatusOptions.map((s) => ({ value: s, label: s }))]"
          @change="applyFilter"
        />
        <UiButton variant="primary" @click="showCreateForm = true">Create Incident</UiButton>
      </div>
    </div>

    <!-- Create form -->
    <div v-if="showCreateForm" class="mb-4 rounded-lg border p-4" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--mnt-text-primary)">New Incident</h3>
      <form @submit.prevent="submitCreate" class="space-y-3">
        <FormField label="Title" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="createForm.title" required :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField label="Severity">
          <template #default="{ id, describedBy, invalid }">
            <SelectInput :id="id" v-model="createForm.severity" :aria-describedby="describedBy" :invalid="invalid" :options="severityOptions.map((s) => ({ value: s, label: s }))" />
          </template>
        </FormField>
        <div>
          <label class="block text-xs font-medium" style="color: var(--mnt-text-secondary)">Affected Components</label>
          <div class="mt-1 max-h-32 space-y-1 overflow-y-auto rounded border p-2" style="border-color: var(--mnt-border-default); background: var(--mnt-bg-elevated)">
            <CheckboxInput
              v-for="c in store.components"
              :key="c.id"
              :value="c.id"
              v-model="createForm.component_ids"
              :label="c.display_name"
            />
            <p v-if="(store.components?.length ?? 0) === 0" class="text-xs" style="color: var(--mnt-text-muted)">No components configured</p>
          </div>
        </div>
        <FormField label="Initial Message" required>
          <template #default="{ id, describedBy, invalid }">
            <TextareaInput :id="id" v-model="createForm.message" required :rows="2" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <div class="flex gap-2">
          <UiButton type="submit" variant="primary">Create</UiButton>
          <UiButton type="button" variant="secondary" @click="resetCreateForm">Cancel</UiButton>
        </div>
      </form>
    </div>

    <!-- Incident list -->
    <div v-if="(store.incidents?.length ?? 0) === 0 && !store.incidentsLoading" class="rounded-lg border p-6 text-center" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <p class="text-sm" style="color: var(--mnt-text-muted)">No incidents</p>
    </div>

    <div class="space-y-3">
      <div
        v-for="inc in store.incidents"
        :key="inc.id"
        class="rounded-lg border p-4"
        style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{
                background: (severityColors[inc.severity] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                color: (severityColors[inc.severity] || { color: 'var(--mnt-text-secondary)' }).color,
              }"
            >
              {{ inc.severity }}
            </span>
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{
                background: (statusBadgeColors[inc.status] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                color: (statusBadgeColors[inc.status] || { color: 'var(--mnt-text-secondary)' }).color,
              }"
            >
              {{ inc.status }}
            </span>
            <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ inc.title }}</span>
          </div>
          <div class="flex items-center gap-2">
            <UiButton v-if="inc.status !== 'resolved'" variant="secondary" size="sm" @click="startPostUpdate(inc)">
              Post Update
            </UiButton>
            <UiButton variant="danger-ghost" size="sm" @click="handleDelete(inc.id)">Delete</UiButton>
          </div>
        </div>

        <!-- Affected components -->
        <div v-if="inc.components?.length" class="mt-1 flex flex-wrap gap-1">
          <span
            v-for="c in inc.components"
            :key="c.component_id"
            class="rounded px-1.5 py-0.5 text-xs"
            style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
          >
            {{ c.name }}
          </span>
        </div>

        <!-- Post update form -->
        <div v-if="showUpdateForm === inc.id" class="mt-3 rounded border p-3" style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-default)">
          <form @submit.prevent="submitUpdate(inc.id)" class="space-y-2">
            <FormField label="Status">
              <template #default="{ id, describedBy, invalid }">
                <SelectInput :id="id" v-model="updateForm.status" :aria-describedby="describedBy" :invalid="invalid" :options="incidentStatusOptions.map((s) => ({ value: s, label: s }))" />
              </template>
            </FormField>
            <FormField label="Message" required>
              <template #default="{ id, describedBy, invalid }">
                <TextareaInput :id="id" v-model="updateForm.message" required :rows="2" :aria-describedby="describedBy" :invalid="invalid" />
              </template>
            </FormField>
            <div class="flex gap-2">
              <UiButton type="submit" variant="primary">Post Update</UiButton>
              <UiButton type="button" variant="secondary" @click="showUpdateForm = null">Cancel</UiButton>
            </div>
          </form>
        </div>

        <!-- Timeline -->
        <div v-if="inc.updates?.length" class="mt-3 border-t pt-2" style="border-color: var(--mnt-border-subtle)">
          <div v-for="u in inc.updates" :key="u.id" class="flex gap-2 py-1">
            <span
              class="rounded px-1.5 py-0.5 text-xs"
              :style="{
                background: (statusBadgeColors[u.status] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                color: (statusBadgeColors[u.status] || { color: 'var(--mnt-text-secondary)' }).color,
              }"
            >
              {{ u.status }}
            </span>
            <span class="flex-1 text-xs" style="color: var(--mnt-text-secondary)">{{ u.message }}</span>
            <span class="text-xs" style="color: var(--mnt-text-muted)">{{ new Date(u.created_at).toLocaleString() }}</span>
          </div>
        </div>

        <p class="mt-1 text-xs" style="color: var(--mnt-text-muted)">Created {{ new Date(inc.created_at).toLocaleString() }}</p>
      </div>
    </div>

    <p v-if="store.incidentsTotal > (store.incidents?.length ?? 0)" class="mt-3 text-center text-xs" style="color: var(--mnt-text-muted)">
      Showing {{ store.incidents?.length ?? 0 }} of {{ store.incidentsTotal }} incidents
    </p>
  </div>
</template>
