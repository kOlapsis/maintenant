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
import type { Endpoint } from '@/services/endpointApi'
import { deleteEndpoint } from '@/services/endpointApi'
import { fetchEndpointDailyUptime, type UptimeDay } from '@/services/uptimeApi'
import { timeAgo } from '@/utils/time'
import EndpointStatusBadge from './EndpointStatusBadge.vue'
import UptimeBar90 from './ui/UptimeBar90.vue'

const props = defineProps<{
  endpoint: Endpoint
}>()

const emit = defineEmits<{
  (e: 'deleted'): void
}>()

const deleting = ref(false)

async function handleDelete() {
  if (!confirm('Delete this endpoint monitor?')) return
  deleting.value = true
  try {
    await deleteEndpoint(props.endpoint.id)
    emit('deleted')
  } catch {
    // silently ignore
  } finally {
    deleting.value = false
  }
}

const uptimeDays = ref<UptimeDay[]>([])

onMounted(async () => {
  try {
    uptimeDays.value = await fetchEndpointDailyUptime(props.endpoint.id)
  } catch {
    // silently ignore - uptime data may not be available
  }
})

const formatTime = (iso: string | undefined) => timeAgo(iso, 'never')

function formatResponseTime(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
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
      transition: 'box-shadow 0.15s ease',
    }"
    class="hover:shadow-pb-elevated"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span
            :style="{
              display: 'inline-flex',
              alignItems: 'center',
              borderRadius: 'var(--pb-radius-sm)',
              padding: '0.125rem 0.375rem',
              fontSize: '0.75rem',
              fontFamily: 'monospace',
              fontWeight: '500',
              textTransform: 'uppercase',
              backgroundColor: endpoint.endpoint_type === 'http' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-warn-bg)',
              color: endpoint.endpoint_type === 'http' ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)',
            }"
          >
            {{ endpoint.endpoint_type }}
          </span>
          <h3
            class="truncate text-sm font-semibold"
            :style="{ color: 'var(--pb-text-primary)' }"
          >
            {{ endpoint.target }}
          </h3>
        </div>
        <p class="mt-0.5 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
          {{ endpoint.source === 'standalone' ? (endpoint.name || 'standalone') : endpoint.container_name }}
        </p>
      </div>
      <div class="ml-2 flex items-center gap-1.5">
        <span
          v-if="endpoint.alert_state === 'alerting'"
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
        <EndpointStatusBadge :status="endpoint.status" />
      </div>
    </div>

    <!-- 90-day uptime bar -->
    <div v-if="uptimeDays.length > 0" class="mt-3">
      <UptimeBar90 :days="uptimeDays" compact />
    </div>

    <div class="mt-3 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <div class="flex items-center gap-3">
        <span v-if="endpoint.last_response_time_ms !== undefined">
          {{ formatResponseTime(endpoint.last_response_time_ms) }}
        </span>
        <span v-if="endpoint.last_http_status">
          HTTP {{ endpoint.last_http_status }}
        </span>
      </div>
      <span>{{ formatTime(endpoint.last_check_at) }}</span>
    </div>

    <div
      v-if="endpoint.last_error && endpoint.status === 'down'"
      class="mt-2 truncate rounded px-2 py-1 text-xs"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        color: 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-sm)',
      }"
    >
      {{ endpoint.last_error }}
    </div>

    <!-- Config summary -->
    <div class="mt-2 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <div class="flex flex-wrap gap-1.5">
        <span>{{ endpoint.config.interval }}</span>
        <span v-if="endpoint.endpoint_type === 'http' && endpoint.config.method !== 'GET'">
          {{ endpoint.config.method }}
        </span>
        <span v-if="endpoint.endpoint_type === 'http' && endpoint.config.expected_status !== '2xx'">
          expect {{ endpoint.config.expected_status }}
        </span>
        <span v-if="endpoint.endpoint_type === 'http' && !endpoint.config.tls_verify" :style="{ color: 'var(--pb-status-warn)' }">
          TLS off
        </span>
      </div>
      <button
        v-if="endpoint.source === 'standalone'"
        class="ml-2 rounded px-1.5 py-0.5 text-xs transition hover:opacity-80"
        :style="{
          backgroundColor: 'var(--pb-status-down-bg)',
          color: 'var(--pb-status-down)',
        }"
        :disabled="deleting"
        @click.stop="handleDelete"
      >
        {{ deleting ? '...' : 'Delete' }}
      </button>
    </div>
  </div>
</template>
