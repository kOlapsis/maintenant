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
import UiButton from '@/components/ui/UiButton.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import RangeInput from '@/components/ui/RangeInput.vue'

const props = defineProps<{
  containerId: string
}>()

const config = ref<ResourceAlertConfig | null>(null)
const cpuThreshold = ref(90)
const memThreshold = ref(90)
const enabled = ref(false)
const saving = ref(false)
const error = ref<string | null>(null)
const saved = ref(false)

const alertStateColors: Record<string, string> = {
  normal: 'text-mnt-status-ok',
  cpu_alert: 'text-mnt-status-down',
  mem_alert: 'text-mnt-status-down',
  both_alert: 'text-mnt-status-down',
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
  <div
    class="rounded border p-4"
    :style="{
      backgroundColor: 'var(--mnt-bg-surface)',
      borderColor: 'var(--mnt-border-default)',
      borderRadius: 'var(--mnt-radius-md)',
    }"
  >
    <div class="mb-3 flex items-center justify-between">
      <h4 class="text-sm font-semibold" :style="{ color: 'var(--mnt-text-secondary)' }">
        Resource Alerts
      </h4>
      <span
        v-if="config"
        class="text-xs font-medium"
        :class="alertStateColors[config.alert_state] || 'text-mnt-muted'"
      >
        {{ config.alert_state }}
      </span>
    </div>

    <div class="space-y-3">
      <!-- Enable toggle -->
      <CheckboxInput v-model="enabled" label="Enable alerts" />

      <!-- CPU threshold -->
      <div>
        <label class="block text-xs text-mnt-muted">CPU Threshold (%)</label>
        <div class="flex items-center gap-2">
          <RangeInput v-model="cpuThreshold" label="CPU Threshold" :min="1" :max="1000" class="flex-1" />
          <TextInput v-model="cpuThreshold" type="number" min="1" max="1000" size="sm" class="w-16" />
        </div>
      </div>

      <!-- Memory threshold -->
      <div>
        <label class="block text-xs text-mnt-muted">Memory Threshold (%)</label>
        <div class="flex items-center gap-2">
          <RangeInput v-model="memThreshold" label="Memory Threshold" :min="1" :max="100" class="flex-1" />
          <TextInput v-model="memThreshold" type="number" min="1" max="100" size="sm" class="w-16" />
        </div>
      </div>

      <!-- Save button -->
      <div class="flex items-center gap-2">
        <UiButton variant="primary" size="sm" :loading="saving" @click="save">
          {{ saving ? 'Saving...' : 'Save' }}
        </UiButton>
        <span v-if="saved" class="text-xs text-mnt-status-ok">Saved</span>
        <span v-if="error" class="text-xs text-mnt-status-down">{{ error }}</span>
      </div>
    </div>
  </div>
</template>
