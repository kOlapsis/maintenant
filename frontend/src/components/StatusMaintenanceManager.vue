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

const store = useStatusAdminStore()

const showForm = ref(false)
const editingId = ref<number | null>(null)
const form = ref({
  title: '',
  description: '',
  starts_at: '',
  ends_at: '',
  component_ids: [] as number[],
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

async function handleDelete(id: number) {
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
  if (mw.active) return { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--pb-accent)' }
  const now = new Date()
  if (new Date(mw.ends_at) < now) return { bg: 'var(--pb-bg-elevated)', color: 'var(--pb-text-muted)' }
  return { bg: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)' }
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Maintenance Windows</h2>
      <button
        @click="showForm = true"
        class="rounded-md px-3 py-1.5 text-sm font-medium text-white"
        style="background: var(--pb-accent)"
      >
        Schedule Maintenance
      </button>
    </div>

    <div v-if="showForm" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">
        {{ editingId ? 'Edit Maintenance' : 'Schedule Maintenance' }}
      </h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Title</label>
          <input v-model="form.title" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Description</label>
          <textarea v-model="form.description" rows="2" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"></textarea>
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Start Time</label>
            <input v-model="form.starts_at" type="datetime-local" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
          </div>
          <div>
            <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">End Time</label>
            <input v-model="form.ends_at" type="datetime-local" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Affected Components</label>
          <div class="mt-1 max-h-32 space-y-1 overflow-y-auto rounded border p-2" style="border-color: var(--pb-border-default); background: var(--pb-bg-elevated)">
            <label v-for="c in store.components" :key="c.id" class="flex items-center gap-2 text-sm" style="color: var(--pb-text-secondary)">
              <input type="checkbox" :value="c.id" v-model="form.component_ids" class="rounded" style="accent-color: var(--pb-accent)" />
              {{ c.display_name }}
            </label>
            <p v-if="(store.components?.length ?? 0) === 0" class="text-xs" style="color: var(--pb-text-muted)">No components configured</p>
          </div>
        </div>
        <div class="flex gap-2">
          <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">
            {{ editingId ? 'Update' : 'Schedule' }}
          </button>
          <button type="button" @click="resetForm" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
        </div>
      </form>
    </div>

    <div v-if="(store.maintenance?.length ?? 0) === 0 && !store.maintenanceLoading" class="rounded-lg border p-6 text-center" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <p class="text-sm" style="color: var(--pb-text-muted)">No maintenance windows scheduled</p>
    </div>

    <div class="space-y-2">
      <div
        v-for="mw in store.maintenance"
        :key="mw.id"
        class="rounded-lg border p-4"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span
              class="rounded px-1.5 py-0.5 text-xs font-medium"
              :style="{ background: statusStyle(mw).bg, color: statusStyle(mw).color }"
            >
              {{ statusLabel(mw) }}
            </span>
            <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ mw.title }}</span>
          </div>
          <div class="flex items-center gap-2">
            <button
              v-if="!mw.active && new Date(mw.ends_at) > new Date()"
              @click="startEdit(mw)"
              class="rounded border px-2 py-1 text-xs"
              style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
            >
              Edit
            </button>
            <button @click="handleDelete(mw.id)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-status-down); color: var(--pb-status-down)">Delete</button>
          </div>
        </div>
        <p v-if="mw.description" class="mt-1 text-xs" style="color: var(--pb-text-muted)">{{ mw.description }}</p>
        <div class="mt-1 text-xs" style="color: var(--pb-text-muted)">
          {{ new Date(mw.starts_at).toLocaleString() }} &mdash; {{ new Date(mw.ends_at).toLocaleString() }}
        </div>
        <div v-if="mw.components?.length" class="mt-1 flex flex-wrap gap-1">
          <span
            v-for="c in mw.components"
            :key="c.component_id"
            class="rounded px-1.5 py-0.5 text-xs"
            style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
          >
            {{ c.name }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
