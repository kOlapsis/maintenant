<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { X } from 'lucide-vue-next'

type Tone = 'neutral' | 'accent' | 'critical' | 'warning'

withDefaults(
  defineProps<{
    pressed?: boolean
    tone?: Tone
    size?: 'md' | 'sm'
    disabled?: boolean
    removable?: boolean
    removeLabel?: string
  }>(),
  { pressed: false, tone: 'neutral', size: 'md', disabled: false, removable: false, removeLabel: 'Remove' },
)

const emit = defineEmits<{ toggle: []; remove: [] }>()

const toneClasses: Record<Tone, { on: string; off: string }> = {
  neutral: {
    on: 'border-mnt-default bg-mnt-elevated text-mnt-secondary',
    off: 'border-mnt-default bg-transparent text-mnt-muted hover:text-mnt-secondary',
  },
  accent: {
    on: 'border-mnt-green-500/30 bg-mnt-green-500/10 text-mnt-green-400 hover:bg-mnt-green-500/20',
    off: 'border-mnt-default bg-transparent text-mnt-muted hover:text-mnt-secondary',
  },
  critical: {
    on: 'border-mnt-status-down/40 bg-mnt-status-down/15 text-mnt-status-down',
    off: 'border-mnt-default bg-transparent text-mnt-muted hover:text-mnt-muted',
  },
  warning: {
    on: 'border-amber-500/40 bg-mnt-status-warn text-mnt-status-warn',
    off: 'border-mnt-default bg-transparent text-mnt-muted hover:text-mnt-muted',
  },
}
</script>

<template>
  <span
    v-if="removable"
    class="inline-flex items-center gap-1 rounded-full border border-mnt-default bg-mnt-elevated px-2 py-0.5 text-xs text-mnt-secondary"
  >
    <slot />
    <button
      type="button"
      class="focus-ring ml-0.5 text-mnt-muted opacity-70 hover:opacity-100"
      :aria-label="removeLabel"
      @click="emit('remove')"
    >
      <X :size="11" aria-hidden="true" />
    </button>
  </span>
  <button
    v-else
    type="button"
    class="focus-ring inline-flex items-center gap-1.5 rounded-full border font-bold transition-all disabled:cursor-not-allowed disabled:opacity-50"
    :class="[
      pressed ? toneClasses[tone].on : toneClasses[tone].off,
      size === 'md' ? 'px-3 py-1.5 text-xs' : 'px-2 py-0.5 text-[10px]',
    ]"
    :aria-pressed="pressed"
    :disabled="disabled"
    @click="emit('toggle')"
  >
    <slot />
  </button>
</template>
