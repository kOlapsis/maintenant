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
import { ref, watch, computed, onUnmounted } from 'vue'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { X } from 'lucide-vue-next'

const props = withDefaults(defineProps<{
  open: boolean
  title?: string
  width?: string
}>(), {
  title: '',
  width: 'max-w-lg',
})

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const panelRef = ref<HTMLElement | null>(null)
const isActive = computed(() => props.open)

useFocusTrap(panelRef, ref(props.open))

watch(() => props.open, (val) => {
  if (val) {
    document.body.style.overflow = 'hidden'
  } else {
    document.body.style.overflow = ''
  }
})

onUnmounted(() => {
  document.body.style.overflow = ''
})

function close() {
  emit('update:open', false)
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    close()
  }
}

function handleOverlayClick() {
  close()
}

const widthMap: Record<string, string> = {
  'max-w-sm': '24rem',
  'max-w-md': '28rem',
  'max-w-lg': '32rem',
  'max-w-xl': '36rem',
  'max-w-2xl': '42rem',
}

const panelWidth = computed(() => widthMap[props.width] || '32rem')
</script>

<template>
  <Teleport to="body">
    <Transition name="slide">
      <div
        v-if="open"
        class="fixed inset-0 z-[9998]"
        @keydown="handleKeydown"
      >
        <!-- Overlay -->
        <div
          class="fixed inset-0 bg-black/60 backdrop-blur-sm"
          @click="handleOverlayClick"
        />

        <!-- Panel -->
        <div
          ref="panelRef"
          class="fixed inset-y-0 right-0 flex flex-col bg-pb-surface shadow-2xl z-[9999]"
          :style="{ width: '100%', maxWidth: panelWidth }"
        >
          <!-- Header -->
          <div class="flex items-center justify-between px-5 py-4 border-b border-slate-800">
            <slot name="header">
              <h2 class="text-lg font-semibold text-pb-primary">{{ title }}</h2>
            </slot>
            <button
              class="flex items-center justify-center w-8 h-8 rounded-lg text-slate-500 hover:text-pb-primary hover:bg-slate-800 transition-colors"
              @click="close"
              aria-label="Close panel"
            >
              <X :size="18" />
            </button>
          </div>

          <!-- Content -->
          <div class="flex-1 overflow-hidden flex flex-col" :class="$slots.header ? '' : 'px-5 py-4 overflow-y-auto'">
            <slot />
          </div>

          <!-- Footer -->
          <div
            v-if="$slots.footer"
            class="px-5 py-4 border-t border-slate-800"
          >
            <slot name="footer" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
@media (max-width: 639px) {
  div[ref="panelRef"] {
    max-width: 100% !important;
  }
}

.slide-enter-active,
.slide-leave-active {
  transition: all 0.3s ease-out;
}

.slide-enter-from,
.slide-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
</style>
