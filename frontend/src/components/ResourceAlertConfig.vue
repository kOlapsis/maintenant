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
import { ref, onMounted } from 'vue'
import {
  getAlertConfig,
  updateAlertConfig,
  type ResourceAlertConfig,
} from '@/services/resourceApi'

const props = defineProps<{
  containerId: number
}>()

const config = ref<ResourceAlertConfig | null>(null)
const cpuThreshold = ref(90)
const memThreshold = ref(90)
const enabled = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const saved = ref(false)

const alertStateColors: Record<string, string> = {
  normal: 'text-green-600',
  cpu_alert: 'text-red-600',
  mem_alert: 'text-red-600',
  both_alert: 'text-red-600',
}

onMounted(async () => {
  try {
    config.value = await getAlertConfig(props.containerId)
    cpuThreshold.value = config.value.cpu_threshold
    memThreshold.value = config.value.mem_threshold
    enabled.value = config.value.enabled
  } catch {
    // Default values already set
  }
})

async function save() {
  saving.value = true
  error.value = null
  saved.value = false
  try {
    config.value = await updateAlertConfig(props.containerId, {
      cpu_threshold: cpuThreshold.value,
      mem_threshold: memThreshold.value,
      enabled: enabled.value,
    })
    saved.value = true
    setTimeout(() => (saved.value = false), 2000)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to save'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="rounded border border-gray-200 bg-white p-4">
    <div class="mb-3 flex items-center justify-between">
      <h4 class="text-sm font-semibold text-slate-700">Resource Alerts</h4>
      <span
        v-if="config"
        class="text-xs font-medium"
        :class="alertStateColors[config.alert_state] || 'text-slate-500'"
      >
        {{ config.alert_state }}
      </span>
    </div>

    <div class="space-y-3">
      <!-- Enable toggle -->
      <label class="flex items-center gap-2 text-sm">
        <input v-model="enabled" type="checkbox" class="rounded border-gray-300" />
        <span class="text-slate-700">Enable alerts</span>
      </label>

      <!-- CPU threshold -->
      <div>
        <label class="block text-xs text-slate-500">CPU Threshold (%)</label>
        <div class="flex items-center gap-2">
          <input
            v-model.number="cpuThreshold"
            type="range"
            min="1"
            max="1000"
            class="flex-1"
          />
          <input
            v-model.number="cpuThreshold"
            type="number"
            min="1"
            max="1000"
            class="w-16 rounded border border-gray-300 px-2 py-1 text-xs"
          />
        </div>
      </div>

      <!-- Memory threshold -->
      <div>
        <label class="block text-xs text-slate-500">Memory Threshold (%)</label>
        <div class="flex items-center gap-2">
          <input
            v-model.number="memThreshold"
            type="range"
            min="1"
            max="100"
            class="flex-1"
          />
          <input
            v-model.number="memThreshold"
            type="number"
            min="1"
            max="100"
            class="w-16 rounded border border-gray-300 px-2 py-1 text-xs"
          />
        </div>
      </div>

      <!-- Save button -->
      <div class="flex items-center gap-2">
        <button
          class="rounded bg-pb-green-600 px-3 py-1.5 text-xs font-medium text-slate-950 hover:bg-pb-green-700 disabled:opacity-50"
          :disabled="saving"
          @click="save"
        >
          {{ saving ? 'Saving...' : 'Save' }}
        </button>
        <span v-if="saved" class="text-xs text-green-600">Saved</span>
        <span v-if="error" class="text-xs text-red-600">{{ error }}</span>
      </div>
    </div>
  </div>
</template>
