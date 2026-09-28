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
