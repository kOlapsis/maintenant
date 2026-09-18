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
import UiTooltip from '@/components/ui/UiTooltip.vue'
import { severityVar } from '@/composables/useSeverity'
import { osPhases, osSupportSeverity, type OSPhase } from '@/utils/osSupport'
import type { AgentOSSupport } from '@/services/agentApi'

const props = defineProps<{
  support: AgentOSSupport
  today?: Date
  compact?: boolean
}>()

const phases = computed(() => osPhases(props.support, props.today))
const summary = computed(() =>
  phases.value.map(p => `${p.label} ${p.status === 'done' ? 'ended' : 'until'} ${p.until}`).join(' · '),
)
const currentColor = computed(() => severityVar(osSupportSeverity(props.support.state)))

function barClass(phase: OSPhase): string {
  if (phase.paid) return 'border border-dashed border-mnt-default'
  if (phase.status === 'done') return phase.key === 'security' ? '' : 'bg-mnt-elevated'
  if (phase.status === 'upcoming') return 'border border-mnt-default'
  return ''
}

function barStyle(phase: OSPhase): Record<string, string> {
  const tinted = (phase.status === 'current' && !phase.paid) || (phase.status === 'done' && phase.key === 'security')
  return tinted ? { backgroundColor: currentColor.value } : {}
}

function labelClass(phase: OSPhase): string {
  if (phase.paid) return 'text-mnt-muted'
  if (phase.status === 'current') return 'text-mnt-primary font-semibold'
  if (phase.status === 'done' && phase.key === 'security') return 'text-mnt-status-down font-semibold'
  return 'text-mnt-muted'
}
</script>

<template>
  <div v-if="compact && phases.length > 0" class="min-w-0">
    <UiTooltip :text="summary" placement="top">
      <div class="flex w-48 max-w-full gap-1 py-1.5" :aria-label="summary">
        <div
          v-for="phase in phases"
          :key="phase.key"
          class="h-1.5 min-w-0 flex-1 rounded-full"
          :class="barClass(phase)"
          :style="barStyle(phase)"
        />
      </div>
    </UiTooltip>
  </div>
  <ol v-else-if="phases.length > 0" class="grid gap-1.5" :style="{ gridTemplateColumns: `repeat(${phases.length}, minmax(0, 1fr))` }">
    <li v-for="phase in phases" :key="phase.key" class="min-w-0">
      <div class="h-1.5 rounded-full" :class="barClass(phase)" :style="barStyle(phase)" aria-hidden="true" />
      <p class="mt-1.5 truncate text-[11px]" :class="labelClass(phase)">{{ phase.label }}</p>
      <p class="truncate font-mono text-[10px] text-mnt-muted">
        {{ phase.status === 'done' ? 'ended' : 'until' }} {{ phase.until }}
      </p>
    </li>
  </ol>
</template>
