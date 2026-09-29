<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed } from 'vue'
import type { ConfirmState } from '@/composables/useConfirm'
import UiModal from './UiModal.vue'
import UiButton from './UiButton.vue'

const props = defineProps<{
  state: ConfirmState | null
}>()

const open = computed({
  get: () => props.state !== null,
  set: (value: boolean) => {
    if (!value) props.state?.resolve(false)
  },
})

function resolve(value: boolean) {
  props.state?.resolve(value)
}
</script>

<template>
  <UiModal v-if="state" v-model:open="open" :title="state.title" size="sm">
    <p class="text-sm leading-relaxed text-mnt-muted">{{ state.message }}</p>
    <template #footer>
      <UiButton variant="secondary" @click="resolve(false)">
        {{ state.cancelLabel || 'Cancel' }}
      </UiButton>
      <UiButton :variant="state.destructive ? 'danger' : 'primary'" @click="resolve(true)">
        {{ state.confirmLabel || 'Confirm' }}
      </UiButton>
    </template>
  </UiModal>
</template>
