<script setup lang="ts">
import CheckIcon from './icons/CheckIcon.vue'
import WarningIcon from './icons/WarningIcon.vue'
import CrossIcon from './icons/CrossIcon.vue'
import PauseIcon from './icons/PauseIcon.vue'
import QuestionIcon from './icons/QuestionIcon.vue'

export type BadgeStatus = 'ok' | 'warning' | 'critical' | 'down' | 'paused' | 'unknown'

const props = withDefaults(
  defineProps<{
    status: BadgeStatus
    size?: 'sm' | 'md' | 'lg'
    label?: string
    showLabel?: boolean
  }>(),
  { size: 'md', showLabel: false },
)

const iconSizes = { sm: 10, md: 14, lg: 18 }
const dotSizes = { sm: 'w-2 h-2', md: 'w-2.5 h-2.5', lg: 'w-3 h-3' }
const textSizes = { sm: 'text-xs', md: 'text-sm', lg: 'text-base' }

const iconComponents: Record<BadgeStatus, typeof CheckIcon> = {
  ok: CheckIcon,
  warning: WarningIcon,
  critical: WarningIcon,
  down: CrossIcon,
  paused: PauseIcon,
  unknown: QuestionIcon,
}

const statusLabels: Record<BadgeStatus, string> = {
  ok: 'Operational',
  warning: 'Warning',
  critical: 'Critical',
  down: 'Down',
  paused: 'Paused',
  unknown: 'Unknown',
}

const statusDotClasses: Record<BadgeStatus, string> = {
  ok: 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]',
  warning: 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.4)]',
  critical: 'bg-orange-500 shadow-[0_0_8px_rgba(249,115,22,0.4)]',
  down: 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.5)]',
  paused: 'bg-slate-500',
  unknown: 'bg-slate-500',
}

const statusTextClasses: Record<BadgeStatus, string> = {
  ok: 'text-emerald-400',
  warning: 'text-amber-400',
  critical: 'text-orange-400',
  down: 'text-rose-400',
  paused: 'text-slate-400',
  unknown: 'text-slate-400',
}

function getLabel() {
  return props.label || statusLabels[props.status]
}
</script>

<template>
  <span class="inline-flex items-center gap-1.5" :class="statusTextClasses[status]">
    <span
      class="inline-flex shrink-0 items-center justify-center rounded-full"
      :class="[dotSizes[size], statusDotClasses[status]]"
    />
    <component :is="iconComponents[status]" :size="iconSizes[size]" />
    <span v-if="showLabel" :class="textSizes[size]">{{ getLabel() }}</span>
    <span class="sr-only">{{ getLabel() }}</span>
  </span>
</template>
