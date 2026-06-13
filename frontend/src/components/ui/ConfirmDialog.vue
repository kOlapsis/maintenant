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
import { watch, ref, nextTick } from 'vue'
import type { ConfirmState } from '@/composables/useConfirm'

const props = defineProps<{
  state: ConfirmState | null
}>()

const cancelRef = ref<HTMLButtonElement | null>(null)

watch(() => props.state, async (s) => {
  if (s) {
    document.body.style.overflow = 'hidden'
    await nextTick()
    cancelRef.value?.focus()
  } else {
    document.body.style.overflow = ''
  }
})

function resolve(value: boolean) {
  props.state?.resolve(value)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    resolve(false)
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="confirm-fade">
      <div
        v-if="state"
        class="fixed inset-0 z-[10001] flex items-center justify-center"
        @keydown="handleKeydown"
      >
        <!-- Overlay -->
        <div
          class="fixed inset-0 bg-black/70 backdrop-blur-sm"
          @click="resolve(false)"
        />

        <!-- Dialog -->
        <div
          class="relative mx-4 w-full max-w-sm overflow-hidden"
          :style="{
            backgroundColor: 'var(--mnt-bg-surface)',
            border: '1px solid var(--mnt-border-default)',
            borderRadius: 'var(--mnt-radius-lg)',
            boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)',
          }"
          role="alertdialog"
          aria-modal="true"
          :aria-labelledby="state.title ? 'confirm-title' : undefined"
          :aria-describedby="state.message ? 'confirm-message' : undefined"
        >
          <div class="p-5">
            <h3
              id="confirm-title"
              class="text-sm font-semibold"
              :style="{ color: 'var(--mnt-text-primary)' }"
            >
              {{ state.title }}
            </h3>
            <p
              id="confirm-message"
              class="mt-2 text-sm leading-relaxed"
              :style="{ color: 'var(--mnt-text-muted)' }"
            >
              {{ state.message }}
            </p>
          </div>

          <div
            class="flex items-center justify-end gap-2 px-5 py-3"
            :style="{ borderTop: '1px solid var(--mnt-border-default)' }"
          >
            <button
              ref="cancelRef"
              class="cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium transition-colors min-h-[36px]"
              :style="{
                color: 'var(--mnt-text-secondary)',
                backgroundColor: 'transparent',
                border: '1px solid var(--mnt-border-default)',
              }"
              @click="resolve(false)"
            >
              {{ state.cancelLabel || 'Cancel' }}
            </button>
            <button
              class="cursor-pointer rounded-lg px-3 py-1.5 text-sm font-medium transition-colors min-h-[36px]"
              :style="{
                color: state.destructive ? '#fff' : 'var(--mnt-text-inverted)',
                backgroundColor: state.destructive ? 'var(--mnt-status-down)' : 'var(--mnt-accent)',
              }"
              @click="resolve(true)"
            >
              {{ state.confirmLabel || 'Confirm' }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.confirm-fade-enter-active {
  transition: opacity 0.15s ease-out;
}
.confirm-fade-leave-active {
  transition: opacity 0.1s ease-in;
}
.confirm-fade-enter-from,
.confirm-fade-leave-to {
  opacity: 0;
}
</style>
