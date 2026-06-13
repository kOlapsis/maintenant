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
import { timeAgo } from '@/utils/time'
import type { EscalationRun } from '@/types/escalation'
import { BellRing, BellOff, CheckCircle2 } from 'lucide-vue-next'

const props = defineProps<{
  runs: EscalationRun[]
}>()

const activeRun = computed(() => props.runs.find((r) => r.status === 'active'))
const latestRun = computed(() => props.runs[0] ?? null)

const displayRun = computed(() => activeRun.value ?? latestRun.value)

const levelLabel = computed(() => {
  if (!displayRun.value) return ''
  if (displayRun.value.status !== 'active') return ''
  const idx = displayRun.value.last_executed_level_index
  return idx >= 0 ? `Level ${idx + 1}` : 'Pending'
})

const statusLabel = computed(() => {
  if (!displayRun.value) return ''
  const s = displayRun.value.status
  const labels: Record<string, string> = {
    stopped_by_ack: 'Stopped (ack)',
    stopped_by_resolution: 'Stopped (resolved)',
    stopped_by_policy_deletion: 'Stopped (policy deleted)',
    stopped_by_edition_downgrade: 'Paused (CE)',
    exhausted: 'Exhausted',
    paused_by_maintenance: 'Paused',
  }
  return labels[s] ?? s
})

// expose timeAgo for template usage
const nextActionLabel = computed(() => {
  if (!displayRun.value?.next_action_at) return 'soon'
  return timeAgo(displayRun.value.next_action_at)
})
</script>

<template>
  <template v-if="displayRun">
    <!-- Active escalation -->
    <div
      v-if="displayRun.status === 'active'"
      class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[10px] font-bold bg-mnt-status-warn border border-amber-500/20 text-mnt-status-warn"
    >
      <BellRing :size="10" class="shrink-0" />
      <span>{{ displayRun.policy?.name ?? 'Escalating' }}</span>
      <span v-if="levelLabel" class="text-mnt-status-warn/70">· {{ levelLabel }}</span>
    </div>
    <!-- Stopped/final status -->
    <div
      v-else
      class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[10px] font-bold bg-mnt-elevated border border-mnt-default text-mnt-muted"
    >
      <CheckCircle2
        v-if="displayRun.status === 'stopped_by_ack' || displayRun.status === 'stopped_by_resolution'"
        :size="10"
        class="shrink-0"
      />
      <BellOff v-else :size="10" class="shrink-0" />
      <span>{{ statusLabel }}</span>
    </div>
  </template>
</template>
