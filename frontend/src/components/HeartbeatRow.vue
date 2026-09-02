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
import type { Heartbeat } from '@/services/heartbeatApi'
import { formatDeadline, formatInterval, heartbeatTone } from '@/utils/heartbeatFormat'
import { timeAgo } from '@/utils/time'
import ListRow from '@/components/ui/ListRow.vue'
import HeartbeatStatusBadge from './HeartbeatStatusBadge.vue'

const props = defineProps<{ heartbeat: Heartbeat }>()

const emit = defineEmits<{ select: [id: string] }>()

// A paused monitor has no deadline to run out, so the countdown is hidden for it.
const deadline = computed(() =>
  props.heartbeat.status === 'paused' || !props.heartbeat.next_deadline_at
    ? null
    : formatDeadline(props.heartbeat.next_deadline_at),
)
</script>

<template>
  <ListRow
    :tone="heartbeatTone(heartbeat.status)"
    :aria-label="`${heartbeat.name}, ${heartbeat.status}`"
    @select="emit('select', heartbeat.id)"
  >
    <!-- Fixed columns, so status badges of different widths do not shift the timings. -->
    <div class="row-grid w-full items-center gap-3">
      <div class="min-w-0">
        <p class="truncate text-sm font-medium text-mnt-primary">{{ heartbeat.name }}</p>
        <p class="truncate text-xs text-mnt-muted">
          every {{ formatInterval(heartbeat.interval_seconds) }} · grace
          {{ formatInterval(heartbeat.grace_seconds) }}
        </p>
      </div>

      <div>
        <HeartbeatStatusBadge :status="heartbeat.status" />
      </div>

      <div class="text-right">
        <p class="truncate text-xs text-mnt-secondary">{{ timeAgo(heartbeat.last_ping_at, 'never') }}</p>
        <p v-if="deadline" class="truncate text-xs text-mnt-muted">{{ deadline }}</p>
      </div>
    </div>
  </ListRow>
</template>

<style scoped>
.row-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 88px 96px;
}

@media (min-width: 640px) {
  .row-grid {
    grid-template-columns: minmax(0, 1fr) 88px 128px;
  }
}
</style>
