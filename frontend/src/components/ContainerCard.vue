<script setup lang="ts">
import type {Container} from '@/services/containerApi'
import {useResourcesStore} from '@/stores/resources'
import {useUpdatesStore} from '@/stores/updates'
import {timeAgo} from '@/utils/time'
import UpdateBadge from '@/components/UpdateBadge.vue'
import {computed} from 'vue'

const props = defineProps<{
  container: Container
}>()

const emit = defineEmits<{
  select: [container: Container]
}>()

const resourcesStore = useResourcesStore()
const updatesStore = useUpdatesStore()

const metrics = computed(() => resourcesStore.formattedSnapshot(props.container.id))
const containerUpdate = computed(() => updatesStore.updates.find(u => u.container_id === props.container.external_id) ?? null)

const stateColors: Record<string, { bg: string; text: string }> = {
  running: { bg: 'var(--pb-status-ok-bg)', text: 'var(--pb-status-ok)' },
  exited: { bg: 'var(--pb-status-down-bg)', text: 'var(--pb-status-down)' },
  completed: { bg: 'var(--pb-bg-elevated)', text: 'var(--pb-text-secondary)' },
  restarting: { bg: 'var(--pb-status-warn-bg)', text: 'var(--pb-status-warn)' },
  paused: { bg: 'var(--pb-bg-elevated)', text: 'var(--pb-accent)' },
  created: { bg: 'var(--pb-bg-elevated)', text: 'var(--pb-text-muted)' },
  dead: { bg: 'var(--pb-status-down-bg)', text: 'var(--pb-status-down)' },
}

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

const formatTime = timeAgo

function getStateStyle(state: string) {
  const colors = stateColors[state] || { bg: 'var(--pb-bg-elevated)', text: 'var(--pb-text-muted)' }
  return {
    backgroundColor: colors.bg,
    color: colors.text,
  }
}
</script>

<template>
  <div
    :style="{
      backgroundColor: 'var(--pb-bg-surface)',
      border: '1px solid var(--pb-border-default)',
      borderRadius: 'var(--pb-radius-lg)',
      padding: '1rem',
      boxShadow: 'var(--pb-shadow-card)',
      transition: 'box-shadow 0.15s ease, border-color 0.15s ease',
      cursor: 'pointer',
    }"
    class="hover:shadow-pb-elevated hover:border-slate-600"
    @click="emit('select', container)"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <h3 class="truncate text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">
            {{ container.name }}
          </h3>
          <span
            v-if="container.has_health_check && container.health_status"
            class="inline-block h-2.5 w-2.5 rounded-full"
            :style="{ backgroundColor: healthColors[container.health_status] || 'var(--pb-text-muted)' }"
            :title="container.health_status"
          />
        </div>
        <div class="mt-0.5 flex items-center gap-2">
          <p class="truncate text-xs" :style="{ color: 'var(--pb-text-muted)' }">
            {{ container.image.split('@')[0] }}
          </p>
          <UpdateBadge :update="containerUpdate" />
        </div>
      </div>
      <div class="ml-2 flex items-center gap-1">
        <span
          v-if="container.state === 'restarting'"
          class="inline-flex items-center rounded-full px-1.5 py-0.5 text-xs font-medium"
          :style="{
            backgroundColor: 'var(--pb-status-critical-bg)',
            color: 'var(--pb-status-critical)',
          }"
          title="Container is restart-looping"
        >
          !!
        </span>
        <span
          class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
          :style="getStateStyle(container.state)"
        >
          {{ container.state }}
        </span>
      </div>
    </div>

    <!-- Resource metrics (running containers only) -->
    <div v-if="container.state === 'running' && metrics" class="mt-3 space-y-1.5">
      <!-- CPU -->
      <div class="flex items-center gap-2 text-xs">
        <span class="w-8" :style="{ color: 'var(--pb-text-muted)' }">CPU</span>
        <div
          class="h-1.5 flex-1 rounded-full"
          :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
        >
          <div
            class="h-1.5 rounded-full transition-all"
            :style="{ width: cpuBarWidth + '%', backgroundColor: barColor(cpuBarWidth) }"
          />
        </div>
        <span class="w-12 text-right" :style="{ color: 'var(--pb-text-secondary)' }">{{ metrics.cpu }}</span>
      </div>
      <!-- Memory -->
      <div class="flex items-center gap-2 text-xs">
        <span class="w-8" :style="{ color: 'var(--pb-text-muted)' }">MEM</span>
        <div
          class="h-1.5 flex-1 rounded-full"
          :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
        >
          <div
            class="h-1.5 rounded-full transition-all"
            :style="{ width: memBarWidth + '%', backgroundColor: barColor(memBarWidth) }"
          />
        </div>
        <span class="w-12 text-right" :style="{ color: 'var(--pb-text-secondary)' }">{{ metrics.memPercent }}</span>
      </div>
      <!-- Network & Block I/O -->
      <div class="flex gap-3 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
        <span>Net: {{ metrics.netRx }}/{{ metrics.netTx }}</span>
        <span>I/O: {{ metrics.blockRead }}/{{ metrics.blockWrite }}</span>
      </div>
    </div>

    <!-- Stopped container -->
    <div v-else-if="container.state !== 'running'" class="mt-3 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      Resources: --
    </div>

    <!-- K8s pod count badge -->
    <div
      v-if="container.runtime_type === 'kubernetes' && container.pod_count && container.pod_count > 0"
      class="mt-2 flex items-center gap-2 text-xs"
      :style="{ color: 'var(--pb-text-muted)' }"
    >
      <span
        v-if="container.controller_kind"
        class="rounded px-1.5 py-0.5"
        :style="{ backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-secondary)' }"
      >{{ container.controller_kind }}</span>
      <span
        :style="{
          color: container.ready_count === container.pod_count ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)',
        }"
      >{{ container.ready_count }}/{{ container.pod_count }} ready</span>
    </div>

    <!-- Error detail -->
    <div
      v-if="container.error_detail"
      class="mt-1 truncate text-xs"
      :style="{ color: 'var(--pb-status-down)' }"
      :title="container.error_detail"
    >{{ container.error_detail }}</div>

    <div class="mt-3 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <span v-if="container.orchestration_unit" class="truncate">
        {{ container.orchestration_unit }}
      </span>
      <span v-else class="truncate">
        {{ container.external_id.slice(0, 12) }}
      </span>
      <span>{{ formatTime(container.last_state_change_at) }}</span>
    </div>
  </div>
</template>
