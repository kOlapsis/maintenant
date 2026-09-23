<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { useToast } from '@/composables/useToast'
import { Info, CheckCircle, AlertTriangle } from 'lucide-vue-next'

const { toasts } = useToast()

const iconMap = {
  info: Info,
  success: CheckCircle,
  warning: AlertTriangle,
} as const
</script>

<template>
  <Teleport to="body">
    <div class="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 pointer-events-none">
      <TransitionGroup
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="opacity-0 translate-y-2 scale-95"
        enter-to-class="opacity-100 translate-y-0 scale-100"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="opacity-100 translate-y-0 scale-100"
        leave-to-class="opacity-0 translate-y-2 scale-95"
      >
        <div
          v-for="toast in toasts"
          :key="toast.id"
          class="toast pointer-events-auto flex items-center gap-3 px-4 py-3 rounded-xl border shadow-2xl shadow-black/40 max-w-sm"
          :class="`toast--${toast.type}`"
        >
          <component :is="iconMap[toast.type]" :size="16" class="toast__icon shrink-0" />
          <div class="toast__msg flex flex-col gap-0.5 text-sm">
            <span v-if="toast.title" class="font-semibold">{{ toast.title }}</span>
            <span :class="toast.title ? '' : 'font-medium'">{{ toast.message }}</span>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast {
  background: var(--mnt-bg-surface);
  backdrop-filter: blur(4px);
}

.toast--info {
  border-color: var(--mnt-border-default);
  color: var(--mnt-text-primary);
}
.toast--info .toast__icon {
  color: var(--mnt-text-muted);
}

.toast--success {
  border-color: var(--mnt-alert-info-border);
  color: var(--mnt-alert-info-title);
}
.toast--success .toast__icon {
  color: var(--mnt-alert-info-icon-color);
}
.toast--success .toast__msg {
  color: var(--mnt-alert-info-text);
}

.toast--warning {
  border-color: var(--mnt-alert-warn-border);
}
.toast--warning .toast__icon {
  color: var(--mnt-alert-warn-icon-color);
}
.toast--warning .toast__msg {
  color: var(--mnt-alert-warn-text);
}
</style>
