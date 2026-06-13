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
import { computed, inject, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAlertsStore } from '@/stores/alerts'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { detailSlideOverKey, type EntityType } from '@/composables/useDetailSlideOver'
import { useEdition } from '@/composables/useEdition'
import type { Alert } from '@/services/alertApi'
import { timeAgo } from '@/utils/time'
import { Activity, ChevronRight } from 'lucide-vue-next'

const { hasFeature } = useEdition()
const router = useRouter()
const detailSlideOver = inject(detailSlideOverKey)!
const alertsStore = useAlertsStore()
const statusAdmin = useStatusAdminStore()

onMounted(() => {
  alertsStore.fetchAlerts()
  alertsStore.fetchActiveAlerts()
  if (hasFeature('incidents')) statusAdmin.fetchIncidents()
})

// Unified incident feed: active alerts + status page incidents
interface IncidentFeedItem {
  id: string
  service: string
  message: string
  time: string
  color: string
  icon: string
  route: string
  entityType: EntityType | null
  entityId: string | null
}

const incidentFeed = computed(() => {
  const items: IncidentFeedItem[] = []

  // Collect all active alerts (deduplicated, sorted by fired_at desc)
  const allActive: Alert[] = [
    ...(alertsStore.activeAlerts.critical ?? []),
    ...(alertsStore.activeAlerts.warning ?? []),
    ...(alertsStore.activeAlerts.info ?? []),
  ].sort((a, b) => new Date(b.fired_at || b.created_at).getTime() - new Date(a.fired_at || a.created_at).getTime())

  for (const alert of allActive.slice(0, 6)) {
    const color =
      alert.severity === 'critical' ? 'bg-pb-sev-incident-solid' :
      alert.severity === 'warning'  ? 'bg-pb-sev-warning-solid' :
      'bg-pb-green-500'
    const route = alertEntityRoute(alert)
    const entityId = alertEntityId(alert)
    const entityType = alertEntityType(alert)
    items.push({
      id: `alert-${alert.id}`,
      service: alert.entity_name || alert.source || `Alert #${alert.id}`,
      message: alert.message,
      time: formatRelativeTime(alert.fired_at || alert.created_at),
      color,
      icon: 'alert',
      route,
      entityType,
      entityId,
    })
  }

  // Active status page incidents (non-resolved)
  for (const inc of statusAdmin.incidents.filter((i) => i.status !== 'resolved').slice(0, 3)) {
    const color =
      inc.severity === 'critical' ? 'bg-pb-sev-incident-solid' :
      inc.severity === 'major'    ? 'bg-pb-sev-incident-solid' :
      inc.severity === 'minor'    ? 'bg-pb-sev-warning-solid' :
      'bg-pb-green-400'
    items.push({
      id: `inc-${inc.id}`,
      service: inc.title,
      message: inc.updates?.[0]?.message ?? `Incident ${inc.status}`,
      time: formatRelativeTime(inc.created_at),
      color,
      icon: 'status',
      route: '/status-admin',
      entityType: null,
      entityId: null,
    })
  }

  return items.slice(0, 6)
})

function alertEntityRoute(alert: Alert): string {
  // Route by source first — some sources (update, security) have entity_type
  // "container" but should navigate to their dedicated page instead.
  switch (alert.source) {
    case 'update': return '/updates'
    case 'security':
      if (alert.entity_type === 'infrastructure') return '/security'
      return hasFeature('security_posture') ? '/security' : '/containers'
  }
  // Default: route by entity type
  switch (alert.entity_type) {
    case 'container': return '/containers'
    case 'endpoint': return '/endpoints'
    case 'heartbeat': return '/heartbeats'
    case 'certificate': return '/certificates'
    default: return '/alerts'
  }
}

function alertEntityId(alert: Alert): string | null {
  const supported = ['container', 'heartbeat', 'certificate']
  if (supported.includes(alert.entity_type) && alert.entity_id) {
    return String(alert.entity_id)
  }
  return null
}

function alertEntityType(alert: Alert): EntityType | null {
  const supported: EntityType[] = ['container', 'heartbeat', 'certificate']
  if (supported.includes(alert.entity_type as EntityType) && alert.entity_id) {
    return alert.entity_type as EntityType
  }
  return null
}

function navigateToIncident(inc: IncidentFeedItem) {
  if (inc.entityType && inc.entityId) {
    detailSlideOver.openDetail(inc.entityType, inc.entityId)
  } else {
    router.push(inc.route)
  }
}

const formatRelativeTime = timeAgo
</script>

<template>
  <div class="bg-pb-surface rounded-xl sm:rounded-2xl border border-pb-default p-4 sm:p-6">
    <div class="flex justify-between items-center mb-5">
      <h3 class="text-sm font-bold text-pb-primary flex items-center gap-2.5">
        <Activity :size="15" class="text-pb-green-500" />
        Incident Activity Feed
      </h3>
      <RouterLink
        to="/alerts"
        class="text-[10px] text-pb-green-500 hover:text-pb-green-400 font-bold uppercase tracking-widest transition-colors"
      >
        View full history
      </RouterLink>
    </div>

    <div v-if="incidentFeed.length > 0" class="space-y-1">
      <div
        v-for="(inc, idx) in incidentFeed"
        :key="inc.id"
        class="flex gap-4 p-3 rounded-xl hover:bg-pb-elevated transition-all border border-transparent hover:border-pb-default/60 group cursor-pointer"
        @click="navigateToIncident(inc)"
      >
        <div class="flex flex-col items-center gap-1 shrink-0">
          <div :class="['w-2 h-2 rounded-full mt-1.5 shrink-0', inc.color]" />
          <div v-if="idx < incidentFeed.length - 1" class="w-px flex-1 bg-pb-elevated" />
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex justify-between items-center mb-0.5">
            <span class="text-xs font-semibold text-pb-primary group-hover:text-pb-green-400 transition-colors tracking-tight truncate mr-3">{{ inc.service }}</span>
            <span class="text-[10px] text-pb-muted font-bold shrink-0">{{ inc.time }}</span>
          </div>
          <p class="text-[11px] text-pb-muted truncate">{{ inc.message }}</p>
        </div>
        <ChevronRight :size="13" class="text-pb-muted group-hover:text-pb-muted self-center shrink-0 transition-colors" />
      </div>
    </div>

    <div v-else class="flex flex-col items-center justify-center py-10 gap-3">
      <div class="w-10 h-10 rounded-full bg-pb-status-ok flex items-center justify-center">
        <Activity :size="18" class="text-pb-status-ok" />
      </div>
      <p class="text-sm text-pb-muted font-medium">No recent incidents</p>
      <p class="text-[10px] text-pb-muted">All services operating normally</p>
    </div>
  </div>
</template>
