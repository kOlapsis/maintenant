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
import { Plus, Minus } from 'lucide-vue-next'

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

function toggleChannel(id: string) {
  const ids = props.modelValue.channel_ids
  const next = ids.includes(id) ? ids.filter((c) => c !== id) : [...ids, id]
  emit('update:modelValue', { ...props.modelValue, channel_ids: next })
}
</script>

<template>
  <div class="bg-pb-primary rounded-xl border border-pb-default p-4 space-y-4">
    <!-- Level header -->
    <div class="flex items-center justify-between">
      <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">
        Level {{ index + 1 }}
      </span>
      <button
        v-if="canRemove"
        class="p-1 rounded text-pb-muted hover:text-pb-status-down hover:bg-pb-status-down/10 transition-all"
        title="Remove level"
        @click="emit('remove')"
      >
        <Minus :size="13" />
      </button>
    </div>

    <!-- Delay -->
    <div class="space-y-2">
      <label class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">
        Trigger after (seconds)
      </label>
      <div class="flex items-center gap-3 flex-wrap">
        <input
          :value="modelValue.delay_seconds"
          type="number"
          min="60"
          max="86400"
          step="60"
          class="w-28 bg-pb-surface border border-pb-default rounded-lg px-3 py-1.5 text-sm text-pb-primary focus:outline-none focus:border-pb-default transition-colors"
          @input="setDelay(Number(($event.target as HTMLInputElement).value))"
        />
        <div class="flex gap-1.5 flex-wrap">
          <button
            v-for="preset in DELAY_PRESETS"
            :key="preset.value"
            class="px-2.5 py-1 rounded text-[10px] font-bold border transition-all"
            :class="
              modelValue.delay_seconds === preset.value
                ? 'bg-pb-elevated border-pb-default text-pb-secondary'
                : 'bg-transparent border-pb-default text-pb-muted hover:border-pb-default hover:text-pb-muted'
            "
            @click="setDelay(preset.value)"
          >
            {{ preset.label }}
          </button>
        </div>
      </div>
    </div>

    <!-- Channels -->
    <div class="space-y-2">
      <label class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Notify via</label>
      <div v-if="channels.length === 0" class="text-xs text-pb-muted">
        No channels available.
      </div>
      <div v-else class="flex flex-wrap gap-2">
        <button
          v-for="ch in channels"
          :key="ch.id"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border transition-all"
          :class="
            modelValue.channel_ids.includes(ch.id)
              ? 'bg-pb-green-500/10 border-pb-green-500/30 text-pb-green-400'
              : 'bg-transparent border-pb-default text-pb-muted hover:border-pb-default hover:text-pb-secondary'
          "
          @click="toggleChannel(ch.id)"
        >
          <Plus v-if="!modelValue.channel_ids.includes(ch.id)" :size="11" />
          <Minus v-else :size="11" />
          {{ ch.name }}
        </button>
      </div>
    </div>
  </div>
</template>
