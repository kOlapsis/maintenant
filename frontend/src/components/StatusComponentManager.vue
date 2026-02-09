<script setup lang="ts">
import { ref } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import {
  createComponent,
  updateComponent,
  deleteComponent,
  type StatusComponent,
} from '@/services/statusApi'

const store = useStatusAdminStore()

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

async function handleDeleteComp(id: number) {
  if (!confirm('Remove this component from the status page?')) return
  await deleteComponent(id)
  store.fetchComponents()
}

async function handleOverride(comp: StatusComponent, status: string | null) {
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
const statusOverrideOptions = ['', 'operational', 'degraded', 'partial_outage', 'major_outage', 'under_maintenance']
</script>

<template>
  <div>
    <!-- Components section -->
    <div>
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Status Components</h2>
        <button
          @click="showCompForm = true"
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
          <div v-if="!editingCompId" class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Monitor Type</label>
              <select v-model="compForm.monitor_type" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
                <option v-for="t in monitorTypes" :key="t" :value="t">{{ t }}</option>
              </select>
            </div>
            <div>
              <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Monitor ID</label>
              <input v-model.number="compForm.monitor_id" type="number" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
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
                  <span v-if="c.status_override" class="rounded px-1.5 py-0.5 text-xs" style="background: rgba(139, 92, 246, 0.15); color: #a78bfa">override: {{ c.status_override }}</span>
                </div>
                <p class="text-xs" style="color: var(--pb-text-muted)">
                  {{ c.monitor_type }}:{{ c.monitor_id }}
                  &middot; {{ c.effective_status }}
                  <span v-if="c.derived_status !== c.effective_status"> (derived: {{ c.derived_status }})</span>
                </p>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <select
                @change="handleOverride(c, ($event.target as HTMLSelectElement).value || null)"
                class="rounded border px-2 py-1 text-xs"
                style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
              >
                <option v-for="s in statusOverrideOptions" :key="s" :value="s" :selected="(c.status_override || '') === s">
                  {{ s || 'No override' }}
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
