<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import { useConfirm } from '@/composables/useConfirm'
import { createSilenceRule, cancelSilenceRule, type CreateSilenceRuleInput } from '@/services/alertApi'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'

const store = useAlertsStore()

const showForm = ref(false)
const form = ref({
  entity_type: '',
  entity_id: undefined as string | undefined,
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

const durationPresetOptions = durationPresets.map((p) => ({ value: String(p.value), label: p.label }))
const durationPresetValue = computed({
  get: () => String(form.value.duration_seconds),
  set: (v: string) => (form.value.duration_seconds = Number(v)),
})

const sourceOptions = [
  { value: '', label: 'All sources (global)' },
  { value: 'container', label: 'Container' },
  { value: 'endpoint', label: 'Endpoint' },
  { value: 'heartbeat', label: 'Heartbeat' },
  { value: 'certificate', label: 'Certificate' },
  { value: 'resource', label: 'Resource' },
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

async function handleCancel(id: string) {
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
      <h2 class="text-lg font-semibold" style="color: var(--mnt-text-primary)">Silence Rules</h2>
      <UiButton variant="primary" @click="showForm = true">Create Silence Rule</UiButton>
    </div>

    <!-- Create form -->
    <div v-if="showForm" class="mb-4 rounded-lg border p-4" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--mnt-text-primary)">New Silence Rule</h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <FormField label="Source (optional)">
          <template #default="{ id, describedBy, invalid }">
            <SelectInput :id="id" v-model="form.source" :options="sourceOptions" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField label="Entity Type (optional)">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.entity_type" placeholder="e.g. container, endpoint" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField label="Entity ID (optional)">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.entity_id" placeholder="Specific entity ID" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <div>
          <label class="block text-xs font-medium" style="color: var(--mnt-text-secondary)">Duration</label>
          <div class="mt-1">
            <SegmentedToggle v-model="durationPresetValue" :options="durationPresetOptions" ariaLabel="Duration preset" />
          </div>
          <div class="mt-2 flex items-center gap-2">
            <TextInput v-model="form.duration_seconds" type="number" min="60" class="w-32" />
            <span class="text-xs" style="color: var(--mnt-text-muted)">seconds</span>
          </div>
        </div>
        <FormField label="Reason (optional)">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.reason" placeholder="e.g. Planned maintenance" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <div class="flex gap-2">
          <UiButton type="submit" variant="primary">Create</UiButton>
          <UiButton type="button" variant="secondary" @click="resetForm">Cancel</UiButton>
        </div>
      </form>
    </div>

    <!-- Rules list -->
    <div class="space-y-2">
      <div
        v-if="store.silenceRules.length === 0 && !store.silenceLoading"
        class="rounded-lg border p-6 text-center"
        style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
      >
        <p class="text-sm" style="color: var(--mnt-text-muted)">No silence rules</p>
      </div>

      <div
        v-for="rule in store.silenceRules"
        :key="rule.id"
        class="rounded-lg border p-3"
        :style="{
          background: 'var(--mnt-bg-surface)',
          borderColor: rule.is_active ? 'var(--mnt-status-warn)' : 'var(--mnt-border-default)',
        }"
      >
        <div class="flex items-center justify-between">
          <div>
            <div class="flex items-center gap-2">
              <span
                class="h-2 w-2 rounded-full"
                :style="{ background: rule.is_active ? 'var(--mnt-status-warn)' : 'var(--mnt-text-muted)' }"
              ></span>
              <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">
                {{ rule.source || rule.entity_type || 'Global' }}
                <span v-if="rule.entity_id" style="color: var(--mnt-text-muted)">#{{ rule.entity_id }}</span>
              </span>
              <span class="rounded px-1.5 py-0.5 text-xs" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-muted)">
                {{ formatDuration(rule.duration_seconds) }}
              </span>
            </div>
            <p v-if="rule.reason" class="mt-0.5 text-xs" style="color: var(--mnt-text-muted)">{{ rule.reason }}</p>
            <p class="text-xs" style="color: var(--mnt-text-muted)">
              {{ formatTime(rule.starts_at) }} - {{ formatTime(rule.expires_at) }}
            </p>
          </div>
          <UiButton v-if="rule.is_active" variant="danger-ghost" size="sm" @click="handleCancel(rule.id)">Cancel</UiButton>
          <span v-else class="text-xs" style="color: var(--mnt-text-muted)">
            {{ rule.cancelled_at ? 'Cancelled' : 'Expired' }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
