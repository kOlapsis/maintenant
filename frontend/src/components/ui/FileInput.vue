<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { ref, watch } from 'vue'
import { Upload, X } from 'lucide-vue-next'
import UiButton from './UiButton.vue'

withDefaults(
  defineProps<{
    accept?: string
    buttonLabel?: string
  }>(),
  { buttonLabel: 'Choose file…' },
)

const file = defineModel<File | null>('file', { default: null })

const inputRef = ref<HTMLInputElement | null>(null)

function onChange(event: Event) {
  file.value = (event.target as HTMLInputElement).files?.[0] ?? null
}

function clear() {
  file.value = null
}

// The native input keeps its own filename even after the model is cleared
// from outside (e.g. the parent resetting the pending selection).
watch(file, (value) => {
  if (!value && inputRef.value) inputRef.value.value = ''
})
</script>

<template>
  <div class="inline-flex flex-wrap items-center gap-2">
    <input ref="inputRef" type="file" :accept="accept" class="sr-only" @change="onChange" />
    <UiButton type="button" variant="secondary" size="sm" :icon="Upload" @click="inputRef?.click()">
      {{ buttonLabel }}
    </UiButton>
    <span v-if="file" class="max-w-[200px] truncate text-xs text-mnt-muted" :title="file.name">
      {{ file.name }}
    </span>
    <UiButton v-if="file" type="button" variant="ghost" size="sm" :icon="X" @click="clear">
      Clear
    </UiButton>
  </div>
</template>
