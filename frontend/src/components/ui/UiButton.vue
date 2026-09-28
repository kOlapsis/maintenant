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
import type { Component } from 'vue'
import { Loader2 } from 'lucide-vue-next'

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'

withDefaults(
  defineProps<{
    variant?: Variant
    size?: 'md' | 'sm'
    type?: 'button' | 'submit' | 'reset'
    loading?: boolean
    disabled?: boolean
    icon?: Component
  }>(),
  {
    variant: 'secondary',
    size: 'md',
    type: 'button',
    loading: false,
    disabled: false,
    icon: undefined,
  },
)

const variantClasses: Record<Variant, string> = {
  primary: 'bg-[var(--mnt-accent)] text-mnt-inverted hover:bg-[var(--mnt-accent-hover)]',
  secondary: 'border border-mnt-default text-mnt-secondary hover:bg-mnt-elevated',
  danger: 'bg-[var(--mnt-status-down)] text-mnt-inverted hover:opacity-90',
  ghost: 'text-mnt-muted hover:bg-mnt-elevated hover:text-mnt-primary',
}
</script>

<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    class="focus-ring inline-flex items-center justify-center gap-1.5 rounded-lg font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50"
    :class="[
      variantClasses[variant],
      size === 'md' ? 'min-h-[44px] px-4 text-sm' : 'min-h-[32px] px-2.5 py-1 text-xs',
    ]"
  >
    <Loader2 v-if="loading" :size="size === 'md' ? 16 : 14" class="animate-spin" aria-hidden="true" />
    <component :is="icon" v-else-if="icon" :size="size === 'md' ? 16 : 14" aria-hidden="true" />
    <slot />
  </button>
</template>
