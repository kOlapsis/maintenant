<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import type { SecurityInsight } from '@/services/securityApi'
import type { RiskAcknowledgment } from '@/services/postureApi'
import { ShieldAlert, Network, Lock, Server, CheckCircle } from 'lucide-vue-next'

const props = defineProps<{
  insights: SecurityInsight[]
  acknowledgments?: RiskAcknowledgment[]
  showAcknowledge?: boolean
}>()

const emit = defineEmits<{
  acknowledge: [insight: SecurityInsight]
  revoke: [ack: RiskAcknowledgment]
}>()

function severityStyle(severity: string) {
  switch (severity) {
    case 'critical':
      return { color: 'var(--pb-status-down)', bg: 'var(--pb-status-down-bg)' }
    case 'high':
      return { color: 'var(--pb-status-warn)', bg: 'var(--pb-status-warn-bg)' }
    case 'medium':
      return { color: '#3b82f6', bg: 'rgba(59,130,246,0.1)' }
    default:
      return { color: 'var(--pb-text-muted)', bg: 'var(--pb-bg-elevated)' }
  }
}

function insightIcon(type: string) {
  switch (type) {
    case 'port_exposed_all_interfaces':
    case 'database_port_exposed':
      return Network
    case 'privileged_container':
      return Lock
    case 'host_network_mode':
      return Server
    case 'service_load_balancer':
    case 'service_node_port':
      return Server
    case 'missing_network_policy':
      return ShieldAlert
    default:
      return ShieldAlert
  }
}

function formatDetail(insight: SecurityInsight): string {
  const parts: string[] = []
  if (insight.details.port) {
    parts.push(`Port ${insight.details.port}`)
  }
  if (insight.details.protocol) {
    parts.push(`${insight.details.protocol}`)
  }
  if (insight.details.database_type) {
    parts.push(`${insight.details.database_type}`)
  }
  return parts.join(' / ')
}

function findingKey(insight: SecurityInsight): string {
  if (insight.details.port) {
    const proto = insight.details.protocol || 'tcp'
    return `${insight.details.port}/${proto}`
  }
  return ''
}

function getAck(insight: SecurityInsight): RiskAcknowledgment | undefined {
  if (!props.acknowledgments) return undefined
  const key = findingKey(insight)
  return props.acknowledgments.find(
    a => a.finding_type === insight.type && a.finding_key === key
  )
}
</script>

<template>
  <div v-if="insights.length > 0">
    <h3 class="mb-3 text-xs font-bold uppercase tracking-wider" :style="{ color: 'var(--pb-text-muted)' }">
      Security Insights
    </h3>
    <div class="space-y-2">
      <div
        v-for="(insight, idx) in insights"
        :key="idx"
        class="flex items-start gap-3 rounded-lg px-3 py-2.5"
        :style="{
          backgroundColor: 'var(--pb-bg-elevated)',
          border: '1px solid var(--pb-border-subtle)',
        }"
      >
        <component
          :is="insightIcon(insight.type)"
          :size="14"
          class="mt-0.5 shrink-0"
          :style="{ color: severityStyle(insight.severity).color }"
        />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="text-xs font-semibold" :style="{ color: 'var(--pb-text-primary)' }">
              {{ insight.title }}
            </span>
            <span
              class="rounded-full px-1.5 py-0.5 text-[10px] font-medium uppercase"
              :style="{
                color: severityStyle(insight.severity).color,
                backgroundColor: severityStyle(insight.severity).bg,
              }"
            >{{ insight.severity }}</span>
            <span
              v-if="getAck(insight)"
              class="flex items-center gap-1 rounded-full bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-400"
            >
              <CheckCircle :size="10" />
              acknowledged
            </span>
          </div>
          <p class="mt-0.5 text-xs" :style="{ color: 'var(--pb-text-secondary)' }">
            {{ insight.description }}
          </p>
          <p
            v-if="formatDetail(insight)"
            class="mt-1 font-mono text-[10px]"
            :style="{ color: 'var(--pb-text-muted)' }"
          >
            {{ formatDetail(insight) }}
          </p>
          <div v-if="showAcknowledge" class="mt-1.5">
            <button
              v-if="!getAck(insight)"
              class="cursor-pointer rounded px-2 py-0.5 text-[10px] font-medium text-slate-400 hover:bg-slate-700/50 hover:text-slate-200 transition-colors"
              @click.stop="emit('acknowledge', insight)"
            >
              Acknowledge
            </button>
            <button
              v-else
              class="cursor-pointer rounded px-2 py-0.5 text-[10px] font-medium text-emerald-400 hover:bg-red-500/10 hover:text-red-400 transition-colors"
              @click.stop="emit('revoke', getAck(insight)!)"
            >
              Revoke
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
