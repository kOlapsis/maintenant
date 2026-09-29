<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { Minus } from 'lucide-vue-next'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'

interface Channel {
  id: string
  name: string
  type: string
  enabled: boolean
}

interface LevelData {
  delay_seconds: number
  channel_ids: string[]
}

const props = defineProps<{
  modelValue: LevelData
  channels: Channel[]
  index: number
  canRemove: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: LevelData]
  remove: []
}>()

const DELAY_PRESETS = [
  { label: '1 min', value: 60 },
  { label: '5 min', value: 300 },
  { label: '15 min', value: 900 },
  { label: '30 min', value: 1800 },
  { label: '1 hour', value: 3600 },
]

function setDelay(v: number) {
  emit('update:modelValue', { ...props.modelValue, delay_seconds: v })
}

function setChannelIds(ids: (string | number)[]) {
  emit('update:modelValue', { ...props.modelValue, channel_ids: ids as string[] })
}
</script>

<template>
  <div class="bg-mnt-primary rounded-xl border border-mnt-default p-4 space-y-4">
    <!-- Level header -->
    <div class="flex items-center justify-between">
      <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
        Level {{ index + 1 }}
      </span>
      <UiButton
        v-if="canRemove"
        variant="ghost"
        size="sm"
        :icon="Minus"
        title="Remove level"
        aria-label="Remove level"
        @click="emit('remove')"
      />
    </div>

    <!-- Delay -->
    <FormField label="Trigger after (seconds)">
      <template #default="{ id, describedBy, invalid }">
        <div class="flex items-center gap-3 flex-wrap">
          <TextInput
            :id="id"
            type="number"
            :model-value="modelValue.delay_seconds"
            min="60"
            max="86400"
            step="60"
            class="w-28"
            :aria-describedby="describedBy"
            :invalid="invalid"
            @update:model-value="(v) => setDelay(Number(v))"
          />
          <SegmentedToggle
            :model-value="String(modelValue.delay_seconds)"
            :options="DELAY_PRESETS.map((p) => ({ value: String(p.value), label: p.label }))"
            ariaLabel="Delay presets"
            @update:model-value="(v) => setDelay(Number(v))"
          />
        </div>
      </template>
    </FormField>

    <!-- Channels -->
    <div class="space-y-2">
      <label class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Notify via</label>
      <div v-if="channels.length === 0" class="text-xs text-mnt-muted">
        No channels available.
      </div>
      <div v-else class="flex flex-wrap gap-4">
        <CheckboxInput
          v-for="ch in channels"
          :key="ch.id"
          :value="ch.id"
          :model-value="modelValue.channel_ids"
          :label="ch.name"
          @update:model-value="(v) => setChannelIds(v as (string | number)[])"
        />
      </div>
    </div>
  </div>
</template>
