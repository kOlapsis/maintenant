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
import type {Container} from '@/services/containerApi'
import {useResourcesStore} from '@/stores/resources'
import {useUpdatesStore} from '@/stores/updates'
import {usePostureStore} from '@/stores/posture'
import {useEdition} from '@/composables/useEdition'
import {timeAgo} from '@/utils/time'
import {getStateStyle as getStateStyleFromUtil} from '@/utils/containerState'
import UpdateBadge from '@/components/UpdateBadge.vue'
import SecurityInsightBadge from '@/components/SecurityInsightBadge.vue'
import PostureScoreBadge from '@/components/PostureScoreBadge.vue'
import {computed, onMounted, ref} from 'vue'

const props = defineProps<{
  container: Container
}>()

const emit = defineEmits<{
  select: [container: Container]
}>()

const resourcesStore = useResourcesStore()
const updatesStore = useUpdatesStore()
const postureStore = usePostureStore()
const { hasFeature } = useEdition()

const metrics = computed(() => resourcesStore.formattedSnapshot(props.container.id))
const containerUpdate = computed(() => updatesStore.updates.find(u => u.container_id === props.container.external_id) ?? null)

const containerScore = ref<{ score: number; color: string } | null>(null)

onMounted(async () => {
  if (hasFeature('security_posture')) {
    const score = await postureStore.fetchContainerScore(props.container.id)
    if (score) {
      containerScore.value = { score: score.score, color: score.color }
    }
  }
})

const healthColors: Record<string, string> = {
  healthy: 'var(--pb-status-ok)',
  unhealthy: 'var(--pb-status-down)',
  starting: 'var(--pb-status-warn)',
}

const cpuBarWidth = computed(() => {
  const snap = resourcesStore.getSnapshot(props.container.id)
  if (!snap) return 0
  return Math.min(snap.cpu_percent, 100)
})

const memBarWidth = computed(() => {
  const snap = resourcesStore.getSnapshot(props.container.id)
  if (!snap || snap.mem_limit === 0) return 0
  return Math.min((snap.mem_used / snap.mem_limit) * 100, 100)
})

function barColor(value: number): string {
  if (value > 80) return 'var(--pb-status-down)'
  if (value > 50) return 'var(--pb-status-warn)'
  return 'var(--pb-status-ok)'
}

const imageTag = computed(() => {
  const base = props.container.image.split('@')[0] ?? props.container.image
  const parts = base.split(':')
  return parts.length > 1 ? parts[parts.length - 1] : base
})

const formatTime = timeAgo

function getStateStyle(state: string) {
  const s = getStateStyleFromUtil(state)
  return {
    backgroundColor: s.bg,
    color: s.color,
  }
}
</script>

<template>
  <div
    class="bg-[#12151C] rounded-xl border border-slate-800 hover:border-slate-700 transition-all cursor-pointer overflow-hidden group"
    @click="emit('select', container)"
  >
    <!-- Header: name + state -->
    <div class="px-4 pt-3.5 pb-2">
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2 min-w-0">
          <span
            v-if="container.has_health_check && container.health_status"
            class="inline-block h-2 w-2 rounded-full shrink-0"
            :style="{ backgroundColor: container.state === 'running' ? (healthColors[container.health_status] || 'var(--pb-text-muted)') : 'var(--pb-text-muted)' }"
            :title="container.state === 'running' ? container.health_status : 'stopped'"
          />
          <h3 class="truncate text-sm font-semibold text-white group-hover:text-pb-green-400 transition-colors">
            {{ container.name }}
          </h3>
        </div>
        <div class="flex items-center gap-1.5 shrink-0">
          <span
            v-if="container.state === 'restarting'"
            class="inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-bold"
            :style="{
              backgroundColor: 'var(--pb-status-critical-bg)',
              color: 'var(--pb-status-critical)',
            }"
            title="Container is restart-looping"
          >!!</span>
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold"
            :style="getStateStyle(container.state)"
          >{{ container.state }}</span>
        </div>
      </div>

      <!-- Badges row: image tag, update, security, posture -->
      <div class="mt-1 flex items-center gap-1.5 flex-wrap">
        <span class="text-[10px] text-slate-500 truncate max-w-[140px]">{{ imageTag }}</span>
        <UpdateBadge :update="containerUpdate" />
        <SecurityInsightBadge
          :count="container.security_insight_count ?? 0"
          :severity="container.security_highest_severity ?? null"
        />
        <PostureScoreBadge
          v-if="containerScore"
          :score="containerScore.score"
          :color="containerScore.color"
          size="xs"
        />
      </div>
    </div>

    <!-- Resource metrics (running containers only) -->
    <div v-if="container.state === 'running' && metrics" class="px-4 pb-1.5 space-y-1">
      <div class="flex items-center gap-2 text-[10px]">
        <span class="w-7 text-slate-600 font-bold uppercase">CPU</span>
        <div class="h-1 flex-1 rounded-full bg-[#0B0E13]">
          <div
            class="h-1 rounded-full transition-all"
            :style="{ width: cpuBarWidth + '%', backgroundColor: barColor(cpuBarWidth) }"
          />
        </div>
        <span class="w-10 text-right text-slate-400 font-mono">{{ metrics.cpu }}</span>
      </div>
      <div class="flex items-center gap-2 text-[10px]">
        <span class="w-7 text-slate-600 font-bold uppercase">MEM</span>
        <div class="h-1 flex-1 rounded-full bg-[#0B0E13]">
          <div
            class="h-1 rounded-full transition-all"
            :style="{ width: memBarWidth + '%', backgroundColor: barColor(memBarWidth) }"
          />
        </div>
        <span class="w-10 text-right text-slate-400 font-mono">{{ metrics.memPercent }}</span>
      </div>
    </div>

    <!-- K8s pod count badge -->
    <div
      v-if="container.runtime_type === 'kubernetes' && container.pod_count && container.pod_count > 0"
      class="px-4 pb-1.5 flex items-center gap-2 text-[10px]"
    >
      <span
        v-if="container.controller_kind"
        class="rounded px-1.5 py-0.5 bg-slate-800 text-slate-400"
      >{{ container.controller_kind }}</span>
      <span
        :style="{
          color: container.ready_count === container.pod_count ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)',
        }"
      >{{ container.ready_count }}/{{ container.pod_count }} ready</span>
    </div>

    <!-- Swarm service info -->
    <div
      v-if="container.controller_kind === 'swarm-service'"
      class="px-4 pb-1.5 flex items-center gap-2 text-[10px]"
    >
      <span class="rounded px-1.5 py-0.5 bg-slate-800 text-slate-400">
        {{ container.swarm_service_mode }}
      </span>
      <span v-if="container.swarm_task_slot" class="text-slate-500">
        slot {{ container.swarm_task_slot }}
      </span>
    </div>

    <!-- Error detail -->
    <div
      v-if="container.error_detail"
      class="px-4 pb-1.5 truncate text-[10px]"
      :style="{ color: 'var(--pb-status-down)' }"
      :title="container.error_detail"
    >{{ container.error_detail }}</div>

    <!-- Footer -->
    <div class="px-4 py-2 flex items-center justify-between text-[10px] text-slate-600 border-t border-slate-800/50">
      <span v-if="container.orchestration_unit" class="truncate font-medium">
        {{ container.orchestration_unit }}
      </span>
      <span v-else class="truncate font-mono">
        {{ container.external_id.slice(0, 12) }}
      </span>
      <span class="shrink-0">{{ formatTime(container.last_state_change_at) }}</span>
    </div>
  </div>
</template>
