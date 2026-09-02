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
import { computed, onMounted, ref } from 'vue'
import type { Endpoint } from '@/services/endpointApi'
import { fetchEndpointDailyUptime, type UptimeDay } from '@/services/uptimeApi'
import { timeAgo } from '@/utils/time'
import { endpointTone } from '@/utils/endpointTone'
import ListRow from '@/components/ui/ListRow.vue'
import UptimeBar90 from '@/components/ui/UptimeBar90.vue'
import EndpointStatusBadge from '@/components/EndpointStatusBadge.vue'

const props = defineProps<{ endpoint: Endpoint }>()

defineEmits<{ select: [] }>()

const tone = computed(() => endpointTone(props.endpoint))

const secondary = computed(() =>
  props.endpoint.source === 'standalone'
    ? props.endpoint.name || 'standalone'
    : props.endpoint.container_name,
)

const uptimeDays = ref<UptimeDay[]>([])

onMounted(async () => {
  try {
    // A row has no space for 90 days; a month reads at this width.
    uptimeDays.value = await fetchEndpointDailyUptime(props.endpoint.id, 30)
  } catch {
    // uptime history is optional decoration on a row
  }
})

function formatResponseTime(ms: number | undefined): string {
  if (ms === undefined || ms === null) return ''
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`
}
</script>

<template>
  <ListRow :tone="tone" @select="$emit('select')">
    <!-- Fixed columns, so a row missing a badge does not shift the ones below it. -->
    <div class="row-grid w-full items-center gap-3">
      <span
        class="rounded px-1 py-0.5 text-center text-[10px] font-semibold uppercase font-mono"
        :style="{
          backgroundColor: endpoint.endpoint_type === 'http' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-warn-bg)',
          color: endpoint.endpoint_type === 'http' ? 'var(--mnt-status-ok-text)' : 'var(--mnt-status-warn-text)',
        }"
      >{{ endpoint.endpoint_type }}</span>

      <div class="min-w-0">
        <p class="truncate text-sm font-medium text-mnt-primary">{{ endpoint.target }}</p>
        <p class="truncate text-xs text-mnt-muted">{{ secondary }}</p>
      </div>

      <div class="flex items-center justify-end gap-1.5">
        <span
          v-if="endpoint.alert_state === 'alerting'"
          class="rounded-full px-2 py-0.5 text-[11px] font-medium"
          :style="{ backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down-text)' }"
        >alerting</span>
        <span
          v-if="endpoint.stale"
          class="rounded-full bg-mnt-sev-unknown px-2 py-0.5 text-[11px] font-medium text-mnt-sev-unknown"
          :title="`Agent offline, last known: ${endpoint.status}`"
        >offline</span>
        <EndpointStatusBadge v-else :status="endpoint.status" />
      </div>

      <div class="col-uptime" title="Uptime, last 30 days">
        <UptimeBar90 v-if="uptimeDays.length > 0" :days="uptimeDays" compact />
      </div>

      <div class="col-response flex items-center justify-end gap-2 font-mono text-xs text-mnt-muted">
        <span class="tabular-nums">{{ formatResponseTime(endpoint.last_response_time_ms) }}</span>
        <span class="w-8 text-right">{{ endpoint.last_http_status || '' }}</span>
      </div>

      <span class="text-right text-xs text-mnt-muted">
        {{ timeAgo(endpoint.last_check_at, 'never') }}
      </span>
    </div>
  </ListRow>
</template>

<style scoped>
.row-grid {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) auto 72px;
}
.col-uptime,
.col-response {
  display: none;
}

@media (min-width: 640px) {
  .row-grid {
    grid-template-columns: 44px minmax(0, 1fr) auto 96px 72px;
  }
  .col-response {
    display: flex;
  }
}

@media (min-width: 1024px) {
  .row-grid {
    grid-template-columns: 44px minmax(0, 1fr) auto 104px 96px 72px;
  }
  .col-uptime {
    display: block;
  }
}
</style>
