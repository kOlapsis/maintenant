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
import { ref, computed, onMounted, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import {
  getEndpoint,
  listChecks,
  deleteEndpoint,
  checkEndpointNow,
  type Endpoint,
  type CheckResult,
} from '@/services/endpointApi'
import { isLocalAgent } from '@/services/apiFetch'
import { fetchEndpointDailyUptime, type UptimeDay } from '@/services/uptimeApi'
import { useEndpointsStore } from '@/stores/endpoints'
import { useConfirm } from '@/composables/useConfirm'
import { timeAgo } from '@/utils/time'
import EndpointStatusBadge from './EndpointStatusBadge.vue'
import EndpointEventTimeline from './EndpointEventTimeline.vue'
import AgentBadge from './AgentBadge.vue'
import UptimeBar90 from './ui/UptimeBar90.vue'

const props = defineProps<{
  endpointId: string
}>()

const emit = defineEmits<{
  close: []
  deleted: []
}>()

const endpointsStore = useEndpointsStore()
const confirm = useConfirm()

const fetched = ref<Endpoint | null>(null)
const checks = ref<CheckResult[]>([])
const uptimeDays = ref<UptimeDay[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const deleting = ref(false)

// Prefer the live store entry (kept fresh by SSE) for status/alert; fall back to
// the fetched snapshot when the store isn't loaded.
const liveEndpoint = computed(() => endpointsStore.endpoints.find(e => e.id === props.endpointId) ?? null)
const endpoint = computed<Endpoint | null>(() => liveEndpoint.value ?? fetched.value)

const isHttp = computed(() => endpoint.value?.endpoint_type === 'http')
const isOffline = computed(() => Boolean(endpoint.value?.stale || endpoint.value?.agent_offline))
const isStandalone = computed(() => endpoint.value?.source === 'standalone')

// Close the panel if the open endpoint disappears (deleted elsewhere via SSE).
const sawInStore = ref(false)
watch(liveEndpoint, (now, prev) => {
  if (now) sawInStore.value = true
  else if (sawInStore.value && prev) emit('close')
})

async function loadData() {
  loading.value = true
  error.value = null
  try {
    const [detail, checksRes, uptime] = await Promise.all([
      getEndpoint(props.endpointId),
      listChecks(props.endpointId, { limit: 100 }),
      fetchEndpointDailyUptime(props.endpointId).catch(() => [] as UptimeDay[]),
    ])
    fetched.value = detail.endpoint
    checks.value = checksRes.checks || []
    uptimeDays.value = uptime
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load endpoint'
  } finally {
    loading.value = false
  }
}

const checking = ref(false)
const checkError = ref<string | null>(null)

// Only endpoints the server probes itself can be re-checked from here: an agent's
// targets sit on its network, and it re-probes them every 30 seconds anyway.
const canCheckNow = computed(() => isLocalAgent(endpoint.value?.agent_id))

async function checkNow() {
  if (checking.value) return
  checking.value = true
  checkError.value = null
  try {
    const res = await checkEndpointNow(props.endpointId)
    fetched.value = res.endpoint
    // A check that changes nothing emits no SSE event, so feed the store directly.
    endpointsStore.applyEndpoint(res.endpoint)
    checks.value = (await listChecks(props.endpointId, { limit: 100 })).checks || []
  } catch (e) {
    checkError.value = e instanceof Error ? e.message : 'Check failed'
  } finally {
    checking.value = false
  }
}

async function handleDelete() {
  const ep = endpoint.value
  if (!ep) return
  const ok = await confirm({
    title: 'Delete endpoint',
    message: `Remove "${ep.name || ep.target}" and all its check history? This cannot be undone.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  deleting.value = true
  try {
    await deleteEndpoint(props.endpointId)
    emit('deleted')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to delete endpoint'
  } finally {
    deleting.value = false
  }
}

function formatRelative(iso: string | undefined): string {
  return timeAgo(iso, 'never')
}

function formatAbsolute(iso: string | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString()
}

function formatResponseTime(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

onMounted(loadData)
watch(() => props.endpointId, () => {
  fetched.value = null
  checks.value = []
  uptimeDays.value = []
  sawInStore.value = false
  loadData()
})
</script>

<template>
  <div class="h-full overflow-y-auto p-5">
    <!-- Header -->
    <div class="mb-4 flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span
            v-if="endpoint"
            :style="{
              display: 'inline-flex',
              alignItems: 'center',
              borderRadius: 'var(--mnt-radius-sm)',
              padding: '0.125rem 0.375rem',
              fontSize: '0.75rem',
              fontFamily: 'monospace',
              fontWeight: '500',
              textTransform: 'uppercase',
              backgroundColor: isHttp ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-warn-bg)',
              color: isHttp ? 'var(--mnt-status-ok)' : 'var(--mnt-status-warn)',
            }"
          >
            {{ endpoint.endpoint_type }}
          </span>
          <h2 class="truncate text-lg font-bold" style="color: var(--mnt-text-primary)">
            {{ endpoint?.name || endpoint?.target || 'Loading…' }}
          </h2>
        </div>
        <p
          v-if="endpoint && endpoint.name"
          class="mt-0.5 break-all font-mono text-xs"
          :style="{ color: 'var(--mnt-text-muted)' }"
        >
          {{ endpoint.target }}
        </p>
      </div>
      <button
        class="shrink-0 rounded px-3 py-1 text-sm transition-colors"
        style="color: var(--mnt-text-muted)"
        @click="emit('close')"
        @mouseenter="($event.target as HTMLElement).style.background = 'var(--mnt-bg-hover)'"
        @mouseleave="($event.target as HTMLElement).style.background = 'transparent'"
      >
        Close
      </button>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="py-8 text-center" style="color: var(--mnt-text-muted)">Loading…</div>

    <!-- Error -->
    <div
      v-else-if="error"
      class="rounded-lg p-4 text-sm"
      :style="{ backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' }"
    >
      {{ error }}
    </div>

    <template v-else-if="endpoint">
      <!-- Status overview -->
      <div class="mb-6 flex flex-wrap items-center gap-3 text-sm">
        <span
          v-if="isOffline"
          class="rounded-full px-2 py-0.5 text-xs font-medium text-mnt-sev-unknown bg-mnt-sev-unknown"
          :title="`Agent offline · last known: ${endpoint.status}`"
        >
          offline
        </span>
        <EndpointStatusBadge v-else :status="endpoint.status" />
        <span
          v-if="endpoint.alert_state === 'alerting'"
          :style="{
            display: 'inline-flex',
            alignItems: 'center',
            borderRadius: '9999px',
            backgroundColor: 'var(--mnt-status-down-bg)',
            color: 'var(--mnt-status-down)',
            padding: '0.125rem 0.5rem',
            fontSize: '0.75rem',
            fontWeight: '500',
          }"
        >
          alerting
        </span>
        <span style="color: var(--mnt-text-muted)">
          Last check: {{ formatRelative(endpoint.last_check_at) }}
        </span>
        <span v-if="endpoint.last_response_time_ms !== undefined" style="color: var(--mnt-text-muted)">
          {{ formatResponseTime(endpoint.last_response_time_ms) }}
        </span>
        <span v-if="isHttp && endpoint.last_http_status" style="color: var(--mnt-text-muted)">
          HTTP {{ endpoint.last_http_status }}
        </span>
        <button
          v-if="canCheckNow"
          type="button"
          class="ml-auto flex cursor-pointer items-center gap-1.5 px-3 py-1 text-xs font-bold transition-all disabled:cursor-default"
          :style="{
            backgroundColor: 'var(--mnt-bg-elevated)',
            color: checking ? 'var(--mnt-text-muted)' : 'var(--mnt-accent)',
            border: '1px solid var(--mnt-border-default)',
            borderRadius: 'var(--mnt-radius-sm)',
          }"
          :disabled="checking"
          title="Probe this endpoint now instead of waiting for the next scheduled check"
          @click="checkNow"
        >
          <RefreshCw :size="12" :class="{ 'animate-spin': checking }" />
          {{ checking ? 'Checking…' : 'Check now' }}
        </button>
      </div>

      <div
        v-if="checkError"
        class="mb-6 break-words rounded-lg px-3 py-2 text-xs"
        :style="{ backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' }"
      >
        {{ checkError }}
      </div>

      <!-- Last error -->
      <div
        v-if="endpoint.last_error && (endpoint.status === 'down' || endpoint.status === 'degraded')"
        class="mb-6 break-words rounded-lg px-3 py-2 text-xs"
        :style="{ backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)' }"
      >
        {{ endpoint.last_error }}
      </div>

      <!-- 90-day uptime -->
      <div v-if="uptimeDays.length > 0" class="mb-6">
        <h3 class="mb-2 text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--mnt-text-muted)' }">
          90-day uptime
        </h3>
        <UptimeBar90 :days="uptimeDays" />
      </div>

      <!-- Event timeline -->
      <div class="mb-6">
        <EndpointEventTimeline :checks="checks" :hours="24" :current-status="endpoint.status" />
      </div>

      <!-- Configuration -->
      <div class="mb-6">
        <h3 class="mb-3 text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--mnt-text-muted)' }">
          Configuration
        </h3>
        <div class="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
          <div>
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Interval</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.interval }}</p>
          </div>
          <div>
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Timeout</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.timeout }}</p>
          </div>
          <div>
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Failure threshold</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.failure_threshold }}</p>
          </div>
          <div>
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Recovery threshold</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.recovery_threshold }}</p>
          </div>
          <div v-if="isHttp && endpoint.config.method">
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Method</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.method }}</p>
          </div>
          <div v-if="isHttp && endpoint.config.expected_status">
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Expected status</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">{{ endpoint.config.expected_status }}</p>
          </div>
          <div v-if="isHttp">
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">TLS verify</span>
            <p
              class="mt-0.5"
              :style="{ color: endpoint.config.tls_verify ? 'var(--mnt-text-primary)' : 'var(--mnt-status-warn)' }"
            >
              {{ endpoint.config.tls_verify ? 'On' : 'Off' }}
            </p>
          </div>
          <div>
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Source</span>
            <p class="mt-0.5" :style="{ color: 'var(--mnt-text-primary)' }">
              {{ isStandalone ? 'Standalone' : `Label · ${endpoint.container_name}` }}
            </p>
          </div>
          <div v-if="endpoint.agent_id">
            <span class="text-xs font-medium" :style="{ color: 'var(--mnt-text-muted)' }">Agent</span>
            <AgentBadge :agent-id="endpoint.agent_id" class="mt-1" />
          </div>
        </div>
      </div>

      <!-- Recent checks -->
      <div class="mb-6">
        <h3 class="mb-3 text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--mnt-text-muted)' }">
          Recent Checks
        </h3>
        <div v-if="checks.length === 0" class="py-4 text-center text-sm" :style="{ color: 'var(--mnt-text-muted)' }">
          No checks recorded yet.
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b text-xs" style="border-color: var(--mnt-border-default); color: var(--mnt-text-muted)">
                <th class="py-2 pr-4">Time</th>
                <th class="py-2 pr-4">Outcome</th>
                <th class="py-2 pr-4">Response</th>
                <th v-if="isHttp" class="py-2 pr-4">Status</th>
                <th class="py-2">Error</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="check in checks"
                :key="check.id"
                class="border-b"
                style="border-color: var(--mnt-border-subtle)"
              >
                <td class="py-2 pr-4 whitespace-nowrap" style="color: var(--mnt-text-secondary)">
                  {{ formatAbsolute(check.timestamp) }}
                </td>
                <td class="py-2 pr-4">
                  <span
                    class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{
                      background: check.success ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-down-bg)',
                      color: check.success ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
                    }"
                  >
                    {{ check.success ? 'up' : 'down' }}
                  </span>
                </td>
                <td class="py-2 pr-4 tabular-nums" style="color: var(--mnt-text-secondary)">
                  {{ formatResponseTime(check.response_time_ms) }}
                </td>
                <td v-if="isHttp" class="py-2 pr-4 tabular-nums" style="color: var(--mnt-text-secondary)">
                  {{ check.http_status ?? '-' }}
                </td>
                <td class="py-2 max-w-[16rem] truncate" style="color: var(--mnt-status-down)" :title="check.error_message">
                  {{ check.error_message || '' }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Delete (standalone only) -->
      <div
        v-if="isStandalone"
        class="flex items-center pt-2"
        :style="{ borderTop: '1px solid var(--mnt-border-subtle)' }"
      >
        <button
          class="ml-auto rounded px-2 py-1 text-xs transition hover:opacity-80"
          :style="{ color: 'var(--mnt-status-down)' }"
          :disabled="deleting"
          @click="handleDelete"
        >
          {{ deleting ? 'Deleting…' : 'Delete endpoint' }}
        </button>
      </div>
    </template>
  </div>
</template>
