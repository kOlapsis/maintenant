<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed } from 'vue'
import AgentBadge from '@/components/AgentBadge.vue'
import OsSupportTimeline from '@/components/OsSupportTimeline.vue'
import StatusDot from '@/components/ui/StatusDot.vue'
import type { AgentOSSupport, OSSupportState } from '@/services/agentApi'
import { osStateTextClass, osSupportLabel, osSupportSeverity, osTimeLeft } from '@/utils/osSupport'

export interface OsSupportRowItem {
  key: string
  label: string
  title?: string
  agent?: { id: string | null | undefined; hostname?: string | null; label?: string | null }
}

const MAX_ITEMS = 6

const props = withDefaults(
  defineProps<{
    state: OSSupportState
    name: string
    title?: string
    support: AgentOSSupport
    items: OsSupportRowItem[]
    hints?: string[]
  }>(),
  { hints: () => [] },
)

const tracked = computed(() => props.state !== 'unknown' && props.state !== 'untracked')
const shown = computed(() => props.items.slice(0, MAX_ITEMS))
const hidden = computed(() => props.items.slice(MAX_ITEMS))
</script>

<template>
  <li
    class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5 px-4 sm:px-5 py-2.5 border-t border-mnt-subtle first:border-t-0 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_12rem_9rem]"
  >
    <div class="flex min-w-0 items-center gap-2.5">
      <StatusDot :severity="osSupportSeverity(state)" :label="osSupportLabel(state)" />
      <p class="truncate text-sm font-bold text-mnt-primary" :title="title">{{ name }}</p>
    </div>

    <p
      v-if="tracked"
      class="text-right text-sm font-black sm:order-last"
      :class="osStateTextClass(state)"
    >{{ osTimeLeft(support) }}</p>

    <ul class="col-span-2 flex min-w-0 flex-wrap gap-1 sm:col-span-1">
      <li
        v-for="item in shown"
        :key="item.key"
        class="flex items-center gap-1 rounded-md bg-mnt-elevated px-1.5 py-0.5 text-[10px] font-semibold text-mnt-secondary"
        :class="item.agent ? 'max-w-full' : 'max-w-[12rem]'"
        :title="item.title"
      >
        <span class="min-w-0 truncate">{{ item.label }}</span>
        <AgentBadge
          v-if="item.agent"
          :agent-id="item.agent.id"
          :hostname="item.agent.hostname"
          :label="item.agent.label"
          show-local
        />
      </li>
      <li
        v-if="hidden.length > 0"
        class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold text-mnt-muted"
        :title="hidden.map((item) => item.label).join(', ')"
      >+{{ hidden.length }}</li>
    </ul>

    <p v-if="state === 'unknown'" class="col-span-2 truncate text-xs text-mnt-secondary" :title="hints.join(' · ')">
      {{ hints.join(' · ') }}
    </p>
    <p v-else-if="state === 'untracked'" class="col-span-2 text-xs text-mnt-muted sm:text-right">
      No published end-of-life dates
    </p>
    <OsSupportTimeline v-else class="col-span-2 sm:col-span-1" :support="support" compact />
  </li>
</template>
