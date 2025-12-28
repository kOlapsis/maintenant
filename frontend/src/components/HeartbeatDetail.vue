<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  getHeartbeat,
  listExecutions,
  type HeartbeatDetailResponse,
  type HeartbeatExecution,
} from '@/services/heartbeatApi'
import HeartbeatStatusBadge from './HeartbeatStatusBadge.vue'
import HeartbeatSnippets from './HeartbeatSnippets.vue'
import HeartbeatGantt from './HeartbeatGantt.vue'
import type { HeartbeatTimelineEntry } from './HeartbeatGantt.vue'
import { formatInterval, nextExpectedPing } from '@/utils/scheduleFormat'

const props = defineProps<{
  heartbeatId: number
}>()

const emit = defineEmits<{
  close: []
}>()

const detail = ref<HeartbeatDetailResponse | null>(null)
const executions = ref<HeartbeatExecution[]>([])
const loading = ref(true)
const expandedPayload = ref<number | null>(null)

onMounted(async () => {
  try {
    const [detailRes, execRes] = await Promise.all([
      getHeartbeat(props.heartbeatId),
      listExecutions(props.heartbeatId, { limit: 20 }),
    ])
    detail.value = detailRes
    executions.value = execRes.executions || []
  } finally {
    loading.value = false
  }
})

function formatTime(iso: string | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString()
}

function formatDuration(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function buildGanttEntries(): HeartbeatTimelineEntry[] {
  // Build timeline entries from executions if available
  if (!detail.value || executions.value.length === 0) return []

  const hb = detail.value.heartbeat
  const interval = hb.interval_seconds || 0
  const grace = hb.grace_seconds || 0

  return executions.value.map(exec => {
    const expectedAt = exec.started_at ?? ''
    const graceDeadline = new Date(new Date(expectedAt || 0).getTime() + grace * 1000).toISOString()
    const receivedAt = exec.completed_at || exec.started_at || null

    let status: 'on-time' | 'late' | 'missed' = 'on-time'
    if (exec.outcome === 'timeout' || !exec.completed_at) {
      status = 'missed'
    } else if (exec.outcome === 'failure') {
      status = 'late'
    }

    return {
      expectedAt,
      receivedAt,
      graceDeadline,
      status,
      duration: exec.duration_ms ?? null,
    }
  })
}

const outcomeColors: Record<string, { bg: string; color: string }> = {
  success: { bg: 'var(--pb-status-ok-bg)', color: 'var(--pb-status-ok)' },
  failure: { bg: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)' },
  timeout: { bg: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)' },
  in_progress: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--pb-accent)' },
}
</script>

<template>
  <div
    class="rounded-lg border p-6"
    style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
  >
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-bold" style="color: var(--pb-text-primary)">
        {{ detail?.heartbeat.name || 'Loading...' }}
      </h2>
      <button
        class="rounded px-3 py-1 text-sm transition-colors"
        style="color: var(--pb-text-muted)"
        @click="emit('close')"
        @mouseenter="($event.target as HTMLElement).style.background = 'var(--pb-bg-hover)'"
        @mouseleave="($event.target as HTMLElement).style.background = 'transparent'"
      >
        Close
      </button>
    </div>

    <div v-if="loading" class="py-8 text-center" style="color: var(--pb-text-muted)">Loading...</div>

    <template v-else-if="detail">
      <!-- Status overview -->
      <div class="mb-6 flex flex-wrap items-center gap-4 text-sm">
        <HeartbeatStatusBadge :status="detail.heartbeat.status" />
        <span style="color: var(--pb-text-muted)">
          Last ping: {{ formatTime(detail.heartbeat.last_ping_at) }}
        </span>
        <span v-if="detail.heartbeat.last_duration_ms" style="color: var(--pb-text-muted)">
          Duration: {{ formatDuration(detail.heartbeat.last_duration_ms) }}
        </span>
        <span v-if="detail.heartbeat.last_exit_code !== undefined" style="color: var(--pb-text-muted)">
          Exit: {{ detail.heartbeat.last_exit_code }}
        </span>
      </div>

      <!-- Schedule explanation -->
      <div
        v-if="detail.heartbeat.interval_seconds"
        class="mb-6 rounded-lg border p-3"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-subtle)"
      >
        <p class="text-sm" style="color: var(--pb-text-secondary)">
          <span class="font-medium" style="color: var(--pb-text-primary)">Schedule:</span>
          {{ formatInterval(detail.heartbeat.interval_seconds) }}
          <span v-if="detail.heartbeat.grace_seconds">
            ({{ detail.heartbeat.grace_seconds }}s grace period)
          </span>
        </p>
        <p v-if="detail.heartbeat.last_ping_at" class="mt-1 text-xs" style="color: var(--pb-text-muted)">
          Next expected: {{ nextExpectedPing(detail.heartbeat.last_ping_at, detail.heartbeat.interval_seconds).toLocaleString() }}
        </p>
      </div>

      <!-- Ping URL -->
      <div
        class="mb-6 rounded-lg p-3"
        style="background: var(--pb-bg-elevated)"
      >
        <p class="mb-1 text-xs font-medium" style="color: var(--pb-text-muted)">Ping URL</p>
        <code class="block break-all text-sm" style="color: var(--pb-text-primary)">
          /ping/{{ detail.heartbeat.uuid }}
        </code>
      </div>

      <!-- Gantt Timeline -->
      <div class="mb-6">
        <HeartbeatGantt :entries="buildGanttEntries()" :hours="48" />
      </div>

      <!-- Snippets -->
      <HeartbeatSnippets
        v-if="detail.snippets"
        :snippets="detail.snippets"
        class="mb-6"
      />

      <!-- Execution history -->
      <div>
        <h3 class="mb-3 text-sm font-semibold" style="color: var(--pb-text-primary)">Execution History</h3>
        <div v-if="executions.length === 0" class="py-4 text-center text-sm" style="color: var(--pb-text-muted)">
          No executions yet
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b text-xs" style="border-color: var(--pb-border-default); color: var(--pb-text-muted)">
                <th class="pb-2 pr-4">Started</th>
                <th class="pb-2 pr-4">Completed</th>
                <th class="pb-2 pr-4">Duration</th>
                <th class="pb-2 pr-4">Exit</th>
                <th class="pb-2 pr-4">Outcome</th>
                <th class="pb-2">Payload</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="exec in executions"
                :key="exec.id"
                class="border-b"
                style="border-color: var(--pb-border-subtle)"
              >
                <td class="py-2 pr-4" style="color: var(--pb-text-secondary)">{{ formatTime(exec.started_at) }}</td>
                <td class="py-2 pr-4" style="color: var(--pb-text-secondary)">{{ formatTime(exec.completed_at) }}</td>
                <td class="py-2 pr-4" style="color: var(--pb-text-secondary)">{{ formatDuration(exec.duration_ms) }}</td>
                <td class="py-2 pr-4" style="color: var(--pb-text-secondary)">{{ exec.exit_code ?? '-' }}</td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{
                      background: (outcomeColors[exec.outcome] || { bg: 'var(--pb-bg-elevated)' }).bg,
                      color: (outcomeColors[exec.outcome] || { color: 'var(--pb-text-secondary)' }).color,
                    }"
                  >
                    {{ exec.outcome }}
                  </span>
                </td>
                <td class="py-2">
                  <button
                    v-if="exec.payload"
                    class="text-xs transition-colors"
                    style="color: var(--pb-accent)"
                    @click="expandedPayload = expandedPayload === exec.id ? null : exec.id"
                  >
                    {{ expandedPayload === exec.id ? 'Hide' : 'Show' }}
                  </button>
                  <span v-else style="color: var(--pb-text-muted)">-</span>
                </td>
              </tr>
              <tr v-for="exec in executions.filter(e => e.payload && expandedPayload === e.id)" :key="'payload-' + exec.id">
                <td colspan="6" class="py-2">
                  <pre
                    class="max-h-40 overflow-auto rounded-lg p-2 font-mono text-xs"
                    style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
                  >{{ exec.payload }}</pre>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>
