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
import { useAlertsStore } from '@/stores/alerts'
import { useConfirm } from '@/composables/useConfirm'
import { createSilenceRule, cancelSilenceRule, type CreateSilenceRuleInput } from '@/services/alertApi'

const store = useAlertsStore()

const showForm = ref(false)
const form = ref({
  entity_type: '',
  entity_id: undefined as number | undefined,
  source: '',
  reason: '',
  duration_seconds: 1800,
})

const durationPresets = [
  { label: '15 min', value: 900 },
  { label: '30 min', value: 1800 },
  { label: '1 hour', value: 3600 },
  { label: '2 hours', value: 7200 },
]

function resetForm() {
  form.value = { entity_type: '', entity_id: undefined, source: '', reason: '', duration_seconds: 1800 }
  showForm.value = false
}

async function submitForm() {
  const data: CreateSilenceRuleInput = { duration_seconds: form.value.duration_seconds }
  if (form.value.entity_type) data.entity_type = form.value.entity_type
  if (form.value.entity_id) data.entity_id = form.value.entity_id
  if (form.value.source) data.source = form.value.source
  if (form.value.reason) data.reason = form.value.reason
  await createSilenceRule(data)
  resetForm()
  store.fetchSilenceRules()
}

const confirm = useConfirm()

async function handleCancel(id: number) {
  const ok = await confirm({
    title: 'Cancel silence rule',
    message: 'Cancel this silence rule? Alerts will resume for affected entities.',
    confirmLabel: 'Cancel rule',
    destructive: true,
  })
  if (!ok) return
  await cancelSilenceRule(id)
  store.fetchSilenceRules()
}

function formatTime(ts: string): string {
  return new Date(ts).toLocaleString()
}

function formatDuration(seconds: number): string {
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  return `${Math.round(seconds / 3600)}h`
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Silence Rules</h2>
      <button
        @click="showForm = true"
        class="rounded-md px-3 py-1.5 text-sm font-medium text-white"
        style="background: var(--pb-accent)"
      >
        Create Silence Rule
      </button>
    </div>

    <!-- Create form -->
    <div v-if="showForm" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">New Silence Rule</h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Source (optional)</label>
          <select v-model="form.source" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
            <option value="">All sources (global)</option>
            <option value="container">Container</option>
            <option value="endpoint">Endpoint</option>
            <option value="heartbeat">Heartbeat</option>
            <option value="certificate">Certificate</option>
            <option value="resource">Resource</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Entity Type (optional)</label>
          <input v-model="form.entity_type" placeholder="e.g. container, endpoint" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Entity ID (optional)</label>
          <input v-model.number="form.entity_id" type="number" placeholder="Specific entity ID" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Duration</label>
          <div class="mt-1 flex flex-wrap gap-2">
            <button
              v-for="preset in durationPresets"
              :key="preset.value"
              type="button"
              @click="form.duration_seconds = preset.value"
              class="rounded-md border px-3 py-1 text-xs transition-colors"
              :style="{
                borderColor: form.duration_seconds === preset.value ? 'var(--pb-accent)' : 'var(--pb-border-default)',
                background: form.duration_seconds === preset.value ? 'rgba(59, 130, 246, 0.15)' : 'transparent',
                color: form.duration_seconds === preset.value ? 'var(--pb-accent)' : 'var(--pb-text-secondary)',
              }"
            >
              {{ preset.label }}
            </button>
          </div>
          <div class="mt-2 flex items-center gap-2">
            <input v-model.number="form.duration_seconds" type="number" min="60" class="w-32 rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
            <span class="text-xs" style="color: var(--pb-text-muted)">seconds</span>
          </div>
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Reason (optional)</label>
          <input v-model="form.reason" placeholder="e.g. Planned maintenance" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div class="flex gap-2">
          <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">Create</button>
          <button type="button" @click="resetForm" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
        </div>
      </form>
    </div>

    <!-- Rules list -->
    <div class="space-y-2">
      <div
        v-if="store.silenceRules.length === 0 && !store.silenceLoading"
        class="rounded-lg border p-6 text-center"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <p class="text-sm" style="color: var(--pb-text-muted)">No silence rules</p>
      </div>

      <div
        v-for="rule in store.silenceRules"
        :key="rule.id"
        class="rounded-lg border p-3"
        :style="{
          background: 'var(--pb-bg-surface)',
          borderColor: rule.is_active ? 'var(--pb-status-warn)' : 'var(--pb-border-default)',
        }"
      >
        <div class="flex items-center justify-between">
          <div>
            <div class="flex items-center gap-2">
              <span
                class="h-2 w-2 rounded-full"
                :style="{ background: rule.is_active ? 'var(--pb-status-warn)' : 'var(--pb-text-muted)' }"
              ></span>
              <span class="text-sm font-medium" style="color: var(--pb-text-primary)">
                {{ rule.source || rule.entity_type || 'Global' }}
                <span v-if="rule.entity_id" style="color: var(--pb-text-muted)">#{{ rule.entity_id }}</span>
              </span>
              <span class="rounded px-1.5 py-0.5 text-xs" style="background: var(--pb-bg-elevated); color: var(--pb-text-muted)">
                {{ formatDuration(rule.duration_seconds) }}
              </span>
            </div>
            <p v-if="rule.reason" class="mt-0.5 text-xs" style="color: var(--pb-text-muted)">{{ rule.reason }}</p>
            <p class="text-xs" style="color: var(--pb-text-muted)">
              {{ formatTime(rule.starts_at) }} - {{ formatTime(rule.expires_at) }}
            </p>
          </div>
          <button
            v-if="rule.is_active"
            @click="handleCancel(rule.id)"
            class="rounded border px-2 py-1 text-xs"
            style="border-color: var(--pb-status-down); color: var(--pb-status-down)"
          >
            Cancel
          </button>
          <span v-else class="text-xs" style="color: var(--pb-text-muted)">
            {{ rule.cancelled_at ? 'Cancelled' : 'Expired' }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
