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
import { ref } from 'vue'
import { X, ChevronDown } from 'lucide-vue-next'

export interface IncidentTimelineEntry {
  id: number
  monitorType: string
  monitorName: string
  severity: 'critical' | 'warning' | 'info'
  message: string
  states: { status: string; timestamp: string; actor: string | null }[]
  isActive: boolean
}

defineProps<{
  incidents: IncidentTimelineEntry[]
}>()

const dismissed = ref(false)
const expandedId = ref<number | null>(null)

const severityDotClasses: Record<string, string> = {
  critical: 'bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.5)]',
  warning: 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.4)]',
  info: 'bg-pb-green-500',
}

const severityBorderClasses: Record<string, string> = {
  critical: 'border-l-rose-500',
  warning: 'border-l-amber-500',
  info: 'border-l-pb-green-500',
}

const timelineSteps = ['detected', 'alerted', 'acknowledged', 'resolved']

function toggleExpand(id: number) {
  expandedId.value = expandedId.value === id ? null : id
}

function getStepTimestamp(incident: IncidentTimelineEntry, step: string): string | null {
  const state = incident.states.find(s => s.status === step)
  return state ? state.timestamp : null
}

function formatTimestamp(ts: string): string {
  const d = new Date(ts)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatFullTimestamp(ts: string): string {
  return new Date(ts).toLocaleString()
}
</script>

<template>
  <div
    v-if="incidents.length > 0 && !dismissed"
    class="bg-pb-surface rounded-2xl border border-slate-800 p-4 mb-4 shadow-lg"
  >
    <div class="flex items-center justify-between mb-3">
      <span class="text-sm font-bold text-rose-400">
        {{ incidents.length }} Active Incident{{ incidents.length > 1 ? 's' : '' }}
      </span>
      <button
        @click="dismissed = true"
        class="p-1.5 rounded-lg text-slate-500 hover:text-pb-primary hover:bg-slate-800 transition-colors"
        aria-label="Dismiss"
      >
        <X :size="14" />
      </button>
    </div>

    <div
      v-for="incident in incidents"
      :key="incident.id"
      class="border-t first:border-t-0 border-slate-800/50"
    >
      <!-- Main incident row -->
      <div
        class="flex items-center gap-3 py-2.5 cursor-pointer rounded-lg transition-colors hover:bg-slate-800/20 border-l-2 pl-3"
        :class="severityBorderClasses[incident.severity] || 'border-l-slate-600'"
        @click="toggleExpand(incident.id)"
      >
        <!-- Severity dot -->
        <span
          class="w-2 h-2 rounded-full shrink-0"
          :class="severityDotClasses[incident.severity] || 'bg-slate-500'"
        />

        <!-- Info -->
        <div class="flex-1 min-w-0">
          <span class="text-sm font-medium text-pb-primary">
            {{ incident.monitorName }}
          </span>
          <span class="text-xs ml-2 text-slate-500">
            {{ incident.message }}
          </span>
        </div>

        <!-- Timeline dots (compact view) -->
        <div class="hidden sm:flex items-center gap-1">
          <template v-for="(step, idx) in timelineSteps" :key="step">
            <span
              class="w-2.5 h-2.5 rounded-full border transition-all"
              :class="
                incident.states.some(s => s.status === step)
                  ? severityDotClasses[incident.severity]
                  : 'bg-transparent border-slate-700'
              "
              :title="step + (getStepTimestamp(incident, step) ? ' at ' + formatFullTimestamp(getStepTimestamp(incident, step)!) : ' (pending)')"
            />
            <span
              v-if="idx < timelineSteps.length - 1"
              class="w-3 h-px"
              :class="
                incident.states.some(s => s.status === step) ? 'bg-slate-500' : 'bg-slate-700'
              "
            />
          </template>
        </div>

        <!-- Expand chevron -->
        <ChevronDown
          :size="14"
          class="shrink-0 text-slate-500 transition-transform"
          :class="{ 'rotate-180': expandedId === incident.id }"
        />
      </div>

      <!-- Expanded timeline detail -->
      <div
        v-if="expandedId === incident.id"
        class="pb-3 pl-8"
      >
        <div class="relative ml-2">
          <!-- Vertical connector line -->
          <div class="absolute left-[5px] top-0 h-full w-px bg-slate-800" />

          <div
            v-for="(step, idx) in timelineSteps"
            :key="step"
            class="relative flex items-start gap-3 pb-3 last:pb-0"
          >
            <!-- Step dot -->
            <div
              class="relative z-10 mt-0.5 w-3 h-3 rounded-full border-2 shrink-0"
              :class="
                incident.states.some(s => s.status === step)
                  ? severityDotClasses[incident.severity]
                  : 'bg-pb-surface border-slate-700'
              "
            />

            <!-- Step details -->
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span
                  class="text-xs font-medium capitalize"
                  :class="incident.states.some(s => s.status === step) ? 'text-pb-primary' : 'text-slate-500'"
                >
                  {{ step }}
                </span>
                <span
                  v-if="getStepTimestamp(incident, step)"
                  class="text-[11px] text-slate-500"
                >
                  {{ formatFullTimestamp(getStepTimestamp(incident, step)!) }}
                </span>
                <span
                  v-else
                  class="text-[11px] italic text-slate-600"
                >
                  pending
                </span>
              </div>
              <p
                v-if="incident.states.find(s => s.status === step)?.actor"
                class="text-[11px] mt-0.5 text-slate-500"
              >
                by {{ incident.states.find(s => s.status === step)!.actor }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
