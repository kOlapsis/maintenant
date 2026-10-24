<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { CreateHeartbeatInput } from '@/services/heartbeatApi'
import { formatInterval } from '@/utils/heartbeatFormat'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import UiButton from '@/components/ui/UiButton.vue'

const props = defineProps<{
  initial?: CreateHeartbeatInput
  submitLabel: string
  busy?: boolean
  error?: string | null
}>()

const emit = defineEmits<{
  submit: [data: CreateHeartbeatInput]
  cancel: []
}>()

const intervalPresets: { label: string; value: number; title?: string }[] = [
  { label: '1m', value: 60 },
  { label: '5m', value: 300 },
  { label: '15m', value: 900 },
  { label: '1h', value: 3600 },
  { label: '6h', value: 21600 },
  { label: '12h', value: 43200 },
  { label: '24h', value: 86400 },
  { label: '7d', value: 604800 },
]

const name = ref(props.initial?.name ?? '')
const interval = ref(props.initial?.interval_seconds ?? 300)
const grace = ref<number | null>(props.initial?.grace_seconds ?? 60)
const attempted = ref(false)

const intervalOptions = (
  intervalPresets.some((p) => p.value === interval.value)
    ? intervalPresets
    : [...intervalPresets, { label: formatInterval(interval.value), value: interval.value, title: `${interval.value} seconds` }]
        .sort((a, b) => a.value - b.value)
).map((p) => ({ value: String(p.value), label: p.label, title: p.title }))

const nameError = computed(() => {
  if (name.value.length > 255) return 'Name must be 255 characters or fewer'
  return attempted.value && name.value === '' ? 'Name is required' : undefined
})

const graceError = computed(() =>
  grace.value === null || grace.value < 0 || grace.value > interval.value
    ? `Grace must be between 0 and ${interval.value} seconds (${formatInterval(interval.value)})`
    : undefined,
)

function handleSubmit() {
  attempted.value = true
  if (nameError.value || graceError.value) return
  emit('submit', { name: name.value, interval_seconds: interval.value, grace_seconds: Number(grace.value) })
}
</script>

<template>
  <form class="flex flex-col gap-3" novalidate @submit.prevent="handleSubmit">
    <FormField label="Name" :error="nameError">
      <template #default="{ id, describedBy, invalid }">
        <TextInput
          :id="id"
          v-model="name"
          placeholder="e.g., Nightly Backup"
          maxlength="255"
          :aria-describedby="describedBy"
          :invalid="invalid"
          required
        />
      </template>
    </FormField>
    <div>
      <span class="mb-1 block text-xs font-medium text-mnt-secondary">Expected interval</span>
      <SegmentedToggle
        :model-value="String(interval)"
        :options="intervalOptions"
        ariaLabel="Expected interval"
        @update:model-value="(v) => (interval = Number(v))"
      />
    </div>
    <FormField label="Grace period (seconds)" :error="graceError">
      <template #default="{ id, describedBy, invalid }">
        <TextInput
          :id="id"
          v-model="grace"
          type="number"
          min="0"
          :max="interval"
          :aria-describedby="describedBy"
          :invalid="invalid"
        />
      </template>
    </FormField>
    <p v-if="error" class="text-sm text-mnt-status-down" role="alert">{{ error }}</p>
    <div class="flex gap-2">
      <UiButton type="submit" variant="primary" :loading="busy">{{ submitLabel }}</UiButton>
      <UiButton variant="ghost" @click="emit('cancel')">Cancel</UiButton>
    </div>
  </form>
</template>
