<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, useId } from 'vue'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { X } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    title: string
    size?: 'sm' | 'md' | 'lg'
    dismissible?: boolean
  }>(),
  { size: 'md', dismissible: true },
)

const open = defineModel<boolean>('open', { default: false })

const titleId = useId()
const panelRef = ref<HTMLElement | null>(null)
const isActive = ref(false)

useFocusTrap(panelRef, isActive)

function applyOpenState(val: boolean) {
  isActive.value = val
  document.body.style.overflow = val ? 'hidden' : ''
}

// An immediate post-flush watcher runs before the first render: the initial open is handled in onMounted.
watch(open, applyOpenState, { flush: 'post' })

onMounted(() => {
  if (open.value) applyOpenState(true)
})

onUnmounted(() => {
  document.body.style.overflow = ''
})

function requestClose() {
  open.value = false
}

function dismiss() {
  if (props.dismissible) requestClose()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') dismiss()
}

const sizeClass: Record<'sm' | 'md' | 'lg', string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-[10001] flex items-center justify-center p-4" @keydown="handleKeydown">
      <div class="fixed inset-0 bg-black/70 backdrop-blur-sm" @click="dismiss" />
      <div
        ref="panelRef"
        class="relative w-full overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface shadow-mnt-elevated"
        :class="sizeClass[size]"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
      >
        <div class="flex items-center justify-between border-b border-mnt-default px-5 py-4">
          <h2 :id="titleId" class="text-sm font-semibold text-mnt-primary">{{ title }}</h2>
          <button
            type="button"
            class="focus-ring flex h-8 w-8 items-center justify-center rounded-lg text-mnt-muted transition-colors hover:bg-mnt-elevated hover:text-mnt-primary"
            aria-label="Close dialog"
            @click="requestClose"
          >
            <X :size="18" aria-hidden="true" />
          </button>
        </div>
        <div class="max-h-[70vh] overflow-y-auto px-5 py-4">
          <slot />
        </div>
        <div v-if="$slots.footer" class="flex items-center justify-end gap-2 border-t border-mnt-default px-5 py-4">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>
