<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  getHeartbeat,
  listExecutions,
  rotatePingToken,
  updateHeartbeat,
  type CreateHeartbeatInput,
  type HeartbeatDetailResponse,
  type HeartbeatExecution,
} from '@/services/heartbeatApi'
import { ApiError } from '@/services/apiFetch'
import HeartbeatStatusBadge from './HeartbeatStatusBadge.vue'
import HeartbeatSnippets from './HeartbeatSnippets.vue'
import HeartbeatGantt from './HeartbeatGantt.vue'
import HeartbeatForm from './HeartbeatForm.vue'
import type { HeartbeatTimelineEntry } from './HeartbeatGantt.vue'
import { formatInterval } from '@/utils/scheduleFormat'
import { useConfirm } from '@/composables/useConfirm'
import UiButton from './ui/UiButton.vue'

const props = defineProps<{
  heartbeatId: string
}>()

const emit = defineEmits<{
  close: []
  refresh: []
}>()

const detail = ref<HeartbeatDetailResponse | null>(null)
const executions = ref<HeartbeatExecution[]>([])
const loading = ref(true)
const expandedPayload = ref<string | null>(null)
const rotating = ref(false)
const rotateError = ref<string | null>(null)
const editing = ref(false)
const saving = ref(false)
const saveError = ref<string | null>(null)
const confirm = useConfirm()

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

async function handleRotate() {
  const ok = await confirm({
    title: 'Rotate ping token',
    message: 'The current ping URL stops working at once. Every job that pings it must switch to the new URL.',
    confirmLabel: 'Rotate',
    destructive: true,
  })
  if (!ok) return
  rotating.value = true
  rotateError.value = null
  try {
    const heartbeat = await rotatePingToken(props.heartbeatId)
    const { snippets } = await getHeartbeat(props.heartbeatId)
    detail.value = { heartbeat, snippets }
    emit('refresh')
  } catch (e) {
    rotateError.value = e instanceof Error ? e.message : 'Failed to rotate ping token'
    // the panel keeps showing the current token
  } finally {
    rotating.value = false
  }
}

async function handleSave(data: CreateHeartbeatInput) {
  saving.value = true
  saveError.value = null
  try {
    const heartbeat = await updateHeartbeat(props.heartbeatId, data)
    detail.value = { ...detail.value, heartbeat }
    editing.value = false
    emit('refresh')
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) saveError.value = 'This heartbeat no longer exists.'
    else saveError.value = e instanceof Error ? e.message : 'Failed to save the heartbeat'
  } finally {
    saving.value = false
  }
}

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
  success: { bg: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok)' },
  failure: { bg: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' },
  timeout: { bg: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)' },
  in_progress: { bg: 'rgba(59, 130, 246, 0.15)', color: 'var(--mnt-accent)' },
}
</script>

<template>
  <div class="h-full overflow-y-auto p-5">
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-bold" style="color: var(--mnt-text-primary)">
        {{ detail?.heartbeat.name || 'Loading...' }}
      </h2>
      <div class="flex items-center gap-2">
        <UiButton v-if="detail && !editing" variant="secondary" size="sm" @click="saveError = null; editing = true">
          Edit
        </UiButton>
        <UiButton variant="ghost" size="sm" @click="emit('close')">
          Close
        </UiButton>
      </div>
    </div>

    <div v-if="loading" class="py-8 text-center" style="color: var(--mnt-text-muted)">Loading...</div>

    <template v-else-if="detail">
      <div
        v-if="editing"
        class="mb-6 rounded-lg border p-3"
        style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-subtle)"
      >
        <HeartbeatForm
          :initial="detail.heartbeat"
          submit-label="Save"
          :busy="saving"
          :error="saveError"
          @submit="handleSave"
          @cancel="editing = false"
        />
      </div>

      <!-- Status overview -->
      <div class="mb-6 flex flex-wrap items-center gap-4 text-sm">
        <HeartbeatStatusBadge :status="detail.heartbeat.status" />
        <span style="color: var(--mnt-text-muted)">
          Last ping: {{ formatTime(detail.heartbeat.last_ping_at) }}
        </span>
        <span v-if="detail.heartbeat.last_duration_ms" style="color: var(--mnt-text-muted)">
          Duration: {{ formatDuration(detail.heartbeat.last_duration_ms) }}
        </span>
        <span v-if="detail.heartbeat.last_exit_code !== undefined" style="color: var(--mnt-text-muted)">
          Exit: {{ detail.heartbeat.last_exit_code }}
        </span>
      </div>

      <!-- Schedule explanation -->
      <div
        v-if="detail.heartbeat.interval_seconds"
        class="mb-6 rounded-lg border p-3"
        style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-subtle)"
      >
        <p class="text-sm" style="color: var(--mnt-text-secondary)">
          <span class="font-medium" style="color: var(--mnt-text-primary)">Schedule:</span>
          {{ formatInterval(detail.heartbeat.interval_seconds) }}
          <span v-if="detail.heartbeat.grace_seconds">
            ({{ detail.heartbeat.grace_seconds }}s grace period)
          </span>
        </p>
        <p v-if="detail.heartbeat.next_deadline_at" class="mt-1 text-xs" style="color: var(--mnt-text-muted)">
          Next expected: {{ formatTime(detail.heartbeat.next_deadline_at) }}
        </p>
      </div>

      <!-- Ping URL -->
      <div
        class="mb-6 rounded-lg p-3"
        style="background: var(--mnt-bg-elevated)"
      >
        <div class="mb-1 flex items-center justify-between gap-2">
          <p class="text-xs font-medium" style="color: var(--mnt-text-muted)">Ping URL</p>
          <UiButton variant="ghost" size="sm" :loading="rotating" @click="handleRotate">
            Rotate token
          </UiButton>
        </div>
        <p v-if="rotateError" class="mb-2 text-xs" style="color: var(--mnt-status-down)">{{ rotateError }}</p>
        <code class="block break-all text-sm" style="color: var(--mnt-text-primary)">
          /ping/{{ detail.heartbeat.ping_token }}
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
        <h3 class="mb-3 text-sm font-semibold" style="color: var(--mnt-text-primary)">Execution History</h3>
        <div v-if="executions.length === 0" class="py-4 text-center text-sm" style="color: var(--mnt-text-muted)">
          No executions yet
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b text-xs" style="border-color: var(--mnt-border-default); color: var(--mnt-text-muted)">
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
                style="border-color: var(--mnt-border-subtle)"
              >
                <td class="py-2 pr-4" style="color: var(--mnt-text-secondary)">{{ formatTime(exec.started_at) }}</td>
                <td class="py-2 pr-4" style="color: var(--mnt-text-secondary)">{{ formatTime(exec.completed_at) }}</td>
                <td class="py-2 pr-4" style="color: var(--mnt-text-secondary)">{{ formatDuration(exec.duration_ms) }}</td>
                <td class="py-2 pr-4" style="color: var(--mnt-text-secondary)">{{ exec.exit_code ?? '-' }}</td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{
                      background: (outcomeColors[exec.outcome] || { bg: 'var(--mnt-bg-elevated)' }).bg,
                      color: (outcomeColors[exec.outcome] || { color: 'var(--mnt-text-secondary)' }).color,
                    }"
                  >
                    {{ exec.outcome }}
                  </span>
                </td>
                <td class="py-2">
                  <UiButton
                    v-if="exec.payload"
                    variant="ghost"
                    size="sm"
                    class="!min-h-0 p-0 text-mnt-accent"
                    @click="expandedPayload = expandedPayload === exec.id ? null : exec.id"
                  >
                    {{ expandedPayload === exec.id ? 'Hide' : 'Show' }}
                  </UiButton>
                  <span v-else style="color: var(--mnt-text-muted)">-</span>
                </td>
              </tr>
              <tr v-for="exec in executions.filter(e => e.payload && expandedPayload === e.id)" :key="'payload-' + exec.id">
                <td colspan="6" class="py-2">
                  <pre
                    class="max-h-40 overflow-auto rounded-lg p-2 font-mono text-xs"
                    style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
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
