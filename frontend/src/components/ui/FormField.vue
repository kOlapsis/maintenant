<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed, useId } from 'vue'

const props = defineProps<{
  label: string
  hint?: string
  error?: string
  required?: boolean
  id?: string
}>()

const autoId = useId()
const fieldId = computed(() => props.id ?? autoId)
const describedBy = computed(() => {
  if (props.error) return `${fieldId.value}-error`
  if (props.hint) return `${fieldId.value}-hint`
  return undefined
})
</script>

<template>
  <div>
    <label :for="fieldId" class="mb-1 block text-xs font-medium text-mnt-secondary">
      {{ label }}<span v-if="required" class="text-mnt-status-down"> *</span>
    </label>
    <slot :id="fieldId" :describedBy="describedBy" :invalid="!!error" />
    <p v-if="error" :id="`${fieldId}-error`" class="mt-1 text-xs text-mnt-status-down">{{ error }}</p>
    <p v-else-if="hint" :id="`${fieldId}-hint`" class="mt-1 text-xs text-mnt-muted">{{ hint }}</p>
  </div>
</template>
