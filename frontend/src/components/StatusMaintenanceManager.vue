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
  createMaintenance,
  updateMaintenance,
  deleteMaintenance,
  type MaintenanceWindow,
} from '@/services/statusApi'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const store = useStatusAdminStore()

const showForm = ref(false)
const editingId = ref<string | null>(null)
const form = ref({
  title: '',
  description: '',
  starts_at: '',
  ends_at: '',
  component_ids: [] as string[],
})

function resetForm() {
  form.value = { title: '', description: '', starts_at: '', ends_at: '', component_ids: [] }
  editingId.value = null
  showForm.value = false
}

function startEdit(mw: MaintenanceWindow) {
  if (mw.active) return
  editingId.value = mw.id
  form.value = {
    title: mw.title,
    description: mw.description,
    starts_at: mw.starts_at.slice(0, 16),
    ends_at: mw.ends_at.slice(0, 16),
    component_ids: mw.components?.map(c => c.component_id) || [],
  }
  showForm.value = true
}

async function submitForm() {
  const data = {
    ...form.value,
    starts_at: new Date(form.value.starts_at).toISOString(),
    ends_at: new Date(form.value.ends_at).toISOString(),
  }
  if (editingId.value) {
    await updateMaintenance(editingId.value, data)
  } else {
    await createMaintenance(data)
  }
  resetForm()
  store.fetchMaintenance()
}

const confirm = useConfirm()

async function handleDelete(id: string) {
  const ok = await confirm({
    title: 'Delete maintenance window',
    message: 'Remove this maintenance window? This cannot be undone.',
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  await deleteMaintenance(id)
  store.fetchMaintenance()
}

function statusLabel(mw: MaintenanceWindow): string {
  if (mw.active) return 'Active'
  const now = new Date()
  if (new Date(mw.ends_at) < now) return 'Completed'
  return 'Upcoming'
}

function statusStyle(mw: MaintenanceWindow): { bg: string; color: string } {
  if (mw.active) return { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--mnt-accent)' }
  const now = new Date()
  if (new Date(mw.ends_at) < now) return { bg: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-muted)' }
  return { bg: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)' }
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--mnt-text-primary)">Maintenance Windows</h2>
      <UiButton variant="primary" @click="showForm = true">Schedule Maintenance</UiButton>
    </div>

    <div v-if="showForm" class="mb-4 rounded-lg border p-4" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--mnt-text-primary)">
        {{ editingId ? 'Edit Maintenance' : 'Schedule Maintenance' }}
      </h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <FormField label="Title" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.title" required :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField label="Description">
          <template #default="{ id, describedBy, invalid }">
            <TextareaInput :id="id" v-model="form.description" :rows="2" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="Start Time" required>
            <template #default="{ id, describedBy, invalid }">
              <TextInput :id="id" v-model="form.starts_at" type="datetime-local" required :aria-describedby="describedBy" :invalid="invalid" />
            </template>
          </FormField>
          <FormField label="End Time" required>
            <template #default="{ id, describedBy, invalid }">
              <TextInput :id="id" v-model="form.ends_at" type="datetime-local" required :aria-describedby="describedBy" :invalid="invalid" />
            </template>
          </FormField>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--mnt-text-secondary)">Affected Components</label>
          <div class="mt-1 max-h-32 space-y-1 overflow-y-auto rounded border p-2" style="border-color: var(--mnt-border-default); background: var(--mnt-bg-elevated)">
            <CheckboxInput
              v-for="c in store.components"
              :key="c.id"
              :value="c.id"
              v-model="form.component_ids"
              :label="c.display_name"
            />
            <p v-if="(store.components?.length ?? 0) === 0" class="text-xs" style="color: var(--mnt-text-muted)">No components configured</p>
          </div>
        </div>
        <div class="flex gap-2">
          <UiButton type="submit" variant="primary">{{ editingId ? 'Update' : 'Schedule' }}</UiButton>
          <UiButton type="button" variant="secondary" @click="resetForm">Cancel</UiButton>
        </div>
      </form>
    </div>

    <div v-if="(store.maintenance?.length ?? 0) === 0 && !store.maintenanceLoading" class="rounded-lg border p-6 text-center" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <p class="text-sm" style="color: var(--mnt-text-muted)">No maintenance windows scheduled</p>
    </div>

    <div class="space-y-2">
      <div
        v-for="mw in store.maintenance"
        :key="mw.id"
        class="rounded-lg border p-4"
        style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{ background: statusStyle(mw).bg, color: statusStyle(mw).color }"
            >
              {{ statusLabel(mw) }}
            </span>
            <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ mw.title }}</span>
          </div>
          <div class="flex items-center gap-2">
            <UiButton
              v-if="!mw.active && new Date(mw.ends_at) > new Date()"
              variant="secondary"
              size="sm"
              @click="startEdit(mw)"
            >
              Edit
            </UiButton>
            <UiButton variant="danger-ghost" size="sm" @click="handleDelete(mw.id)">Delete</UiButton>
          </div>
        </div>
        <p v-if="mw.description" class="mt-1 text-xs" style="color: var(--mnt-text-muted)">{{ mw.description }}</p>
        <div class="mt-1 text-xs" style="color: var(--mnt-text-muted)">
          {{ new Date(mw.starts_at).toLocaleString() }} &mdash; {{ new Date(mw.ends_at).toLocaleString() }}
        </div>
        <div v-if="mw.components?.length" class="mt-1 flex flex-wrap gap-1">
          <span
            v-for="c in mw.components"
            :key="c.component_id"
            class="rounded px-1.5 py-0.5 text-xs"
            style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
          >
            {{ c.name }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
