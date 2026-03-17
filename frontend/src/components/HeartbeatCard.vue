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
import { ref, onMounted } from 'vue'
import type { Heartbeat } from '@/services/heartbeatApi'
import { deleteHeartbeat, pauseHeartbeat, resumeHeartbeat } from '@/services/heartbeatApi'
import { fetchHeartbeatDailyUptime, type UptimeDay } from '@/services/uptimeApi'
import { useConfirm } from '@/composables/useConfirm'
import { timeAgo } from '@/utils/time'
import HeartbeatStatusBadge from './HeartbeatStatusBadge.vue'
import UptimeBar90 from './ui/UptimeBar90.vue'

const props = defineProps<{
  heartbeat: Heartbeat
}>()

const emit = defineEmits<{
  refresh: []
  select: [id: number]
}>()

const copied = ref(false)
const uptimeDays = ref<UptimeDay[]>([])

onMounted(async () => {
  try {
    uptimeDays.value = await fetchHeartbeatDailyUptime(props.heartbeat.id)
  } catch {
    // silently ignore
  }
})

const formatTime = (iso: string | undefined) => timeAgo(iso, 'never')

function formatInterval(seconds: number): string {
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`
  return `${Math.floor(seconds / 86400)}d`
}

function formatDuration(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

async function copyPingUrl() {
  const url = `${window.location.origin}/ping/${props.heartbeat.uuid}`
  await navigator.clipboard.writeText(url)
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}

async function handlePause() {
  await pauseHeartbeat(props.heartbeat.id)
  emit('refresh')
}

async function handleResume() {
  await resumeHeartbeat(props.heartbeat.id)
  emit('refresh')
}

const confirm = useConfirm()
const deleting = ref(false)

async function handleDelete() {
  const ok = await confirm({
    title: 'Delete heartbeat',
    message: `Remove "${props.heartbeat.name}" and all its history? This cannot be undone.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  deleting.value = true
  try {
    await deleteHeartbeat(props.heartbeat.id)
    emit('refresh')
  } catch {
    // silently ignore
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div
    class="cursor-pointer"
    :style="{
      backgroundColor: 'var(--pb-bg-surface)',
      border: '1px solid var(--pb-border-default)',
      borderRadius: 'var(--pb-radius-lg)',
      padding: '1rem',
      boxShadow: 'var(--pb-shadow-card)',
      transition: 'box-shadow 0.15s ease',
    }"
    @click="emit('select', heartbeat.id)"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0 flex-1">
        <h3 class="truncate text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">
          {{ heartbeat.name }}
        </h3>
        <p class="mt-0.5 truncate font-mono text-xs" :style="{ color: 'var(--pb-text-muted)' }">
          /ping/{{ heartbeat.uuid.slice(0, 8) }}...
        </p>
      </div>
      <div class="ml-2 flex items-center gap-1.5">
        <span
          v-if="heartbeat.alert_state === 'alerting'"
          :style="{
            display: 'inline-flex',
            alignItems: 'center',
            borderRadius: '9999px',
            backgroundColor: 'var(--pb-status-down-bg)',
            color: 'var(--pb-status-down)',
            padding: '0.125rem 0.375rem',
            fontSize: '0.75rem',
            fontWeight: '500',
          }"
        >
          alerting
        </span>
        <HeartbeatStatusBadge :status="heartbeat.status" />
      </div>
    </div>

    <!-- 90-day uptime bar -->
    <div v-if="uptimeDays.length > 0" class="mt-3">
      <UptimeBar90 :days="uptimeDays" compact />
    </div>

    <div class="mt-3 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <div class="flex items-center gap-3">
        <span>every {{ formatInterval(heartbeat.interval_seconds) }}</span>
        <span>grace {{ formatInterval(heartbeat.grace_seconds) }}</span>
        <span v-if="heartbeat.last_duration_ms !== undefined">
          {{ formatDuration(heartbeat.last_duration_ms) }}
        </span>
      </div>
      <span>{{ formatTime(heartbeat.last_ping_at) }}</span>
    </div>

    <!-- Actions -->
    <div
      class="mt-3 flex items-center gap-2 pt-2"
      :style="{ borderTop: '1px solid var(--pb-border-subtle)' }"
      @click.stop
    >
      <button
        class="rounded px-2 py-0.5 text-xs"
        :style="{ color: 'var(--pb-text-secondary)' }"
        @click="copyPingUrl"
      >
        {{ copied ? 'Copied!' : 'Copy URL' }}
      </button>
      <button
        v-if="heartbeat.status !== 'paused'"
        class="rounded px-2 py-0.5 text-xs"
        :style="{ color: 'var(--pb-status-warn)' }"
        @click="handlePause"
      >
        Pause
      </button>
      <button
        v-else
        class="rounded px-2 py-0.5 text-xs"
        :style="{ color: 'var(--pb-status-ok)' }"
        @click="handleResume"
      >
        Resume
      </button>
      <button
        class="ml-auto rounded px-2 py-0.5 text-xs transition hover:opacity-80"
        :style="{ color: 'var(--pb-status-down)' }"
        :disabled="deleting"
        @click="handleDelete"
      >
        {{ deleting ? 'Deleting...' : 'Delete' }}
      </button>
    </div>
  </div>
</template>
