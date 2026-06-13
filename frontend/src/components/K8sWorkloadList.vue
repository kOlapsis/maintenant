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
import { type K8sWorkloadGroup, type K8sWorkload } from '@/services/kubernetesApi'
import { timeAgo } from '@/utils/time'
import { ChevronDown, ChevronRight } from 'lucide-vue-next'
import HostBadge from '@/components/HostBadge.vue'

defineProps<{
  groups: K8sWorkloadGroup[]
}>()

const emit = defineEmits<{
  select: [workload: K8sWorkload]
}>()

const expandedGroups = ref<Set<string>>(new Set())

function toggleGroup(ns: string) {
  if (expandedGroups.value.has(ns)) {
    expandedGroups.value.delete(ns)
  } else {
    expandedGroups.value.add(ns)
  }
}

function isExpanded(ns: string): boolean {
  return expandedGroups.value.has(ns)
}

function ensureExpanded(groups: K8sWorkloadGroup[]) {
  for (const g of groups) {
    if (!expandedGroups.value.has(g.namespace)) {
      expandedGroups.value.add(g.namespace)
    }
  }
}

function statusStyle(status: K8sWorkload['status']): string {
  switch (status) {
    case 'healthy':
      return 'text-mnt-status-ok bg-mnt-status-ok border-emerald-400/20'
    case 'degraded':
      return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
    case 'progressing':
      return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
    case 'failed':
      return 'text-mnt-status-down bg-mnt-status-down border-mnt-sev-incident'
  }
}

function replicaColor(ready: number, desired: number): string {
  if (ready >= desired && desired > 0) return 'text-mnt-status-ok'
  if (ready > 0) return 'text-mnt-status-warn'
  return 'text-mnt-status-down'
}

function kindStyle(kind: K8sWorkload['kind']): string {
  switch (kind) {
    case 'Deployment':
      return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
    case 'StatefulSet':
      return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
    case 'DaemonSet':
      return 'text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning'
    case 'Job':
      return 'text-mnt-muted bg-mnt-elevated border-mnt-default'
  }
}

function primaryImage(images: string[]): { name: string; tag: string } {
  const image = images[0] ?? ''
  const colonIdx = image.lastIndexOf(':')
  if (colonIdx < 0) return { name: image, tag: 'latest' }
  return { name: image.slice(0, colonIdx), tag: image.slice(colonIdx + 1) }
}

// Ensure groups expand by default when data arrives — called from parent via watch or v-if
function handleGroupsReady(groups: K8sWorkloadGroup[]) {
  ensureExpanded(groups)
}
</script>

<template>
  <div class="space-y-4" @vue:mounted="handleGroupsReady(groups)">
    <div
      v-for="group in groups"
      :key="group.namespace"
      class="bg-mnt-surface rounded-xl border border-mnt-default overflow-hidden"
    >
      <!-- Namespace header -->
      <button
        class="w-full flex items-center justify-between px-4 py-3 hover:bg-mnt-elevated transition-all"
        @click="toggleGroup(group.namespace)"
      >
        <div class="flex items-center gap-3">
          <component
            :is="isExpanded(group.namespace) ? ChevronDown : ChevronRight"
            :size="14"
            class="text-mnt-muted flex-shrink-0"
          />
          <span class="text-sm font-semibold text-mnt-primary font-mono">{{ group.namespace }}</span>
          <span
            class="text-[10px] font-bold uppercase tracking-wider text-mnt-muted bg-mnt-elevated border border-mnt-default px-1.5 py-0.5 rounded"
          >
            {{ group.workloads.length }} workload{{ group.workloads.length === 1 ? '' : 's' }}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <span
            :class="[
              'text-xs font-semibold tabular-nums',
              group.workloads.every((w) => w.status === 'healthy')
                ? 'text-mnt-status-ok'
                : group.workloads.some((w) => w.status === 'failed')
                  ? 'text-mnt-status-down'
                  : 'text-mnt-status-warn',
            ]"
          >
            {{ group.workloads.filter((w) => w.status === 'healthy').length }}/{{
              group.workloads.length
            }}
            healthy
          </span>
        </div>
      </button>

      <!-- Workload rows -->
      <div
        v-if="isExpanded(group.namespace)"
        class="border-t border-mnt-default divide-y divide-slate-800/60"
      >
        <div
          v-for="workload in group.workloads"
          :key="workload.id"
          class="px-4 py-3 hover:bg-mnt-elevated transition-all cursor-pointer group"
          @click="emit('select', workload)"
        >
          <div class="flex items-center justify-between gap-4">
            <!-- Left: name + badges -->
            <div class="flex items-center gap-2 min-w-0">
              <span
                class="text-sm text-mnt-primary font-medium truncate group-hover:text-mnt-green-400 transition-colors"
              >
                {{ workload.name }}
              </span>
              <span
                :class="[
                  'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0',
                  kindStyle(workload.kind),
                ]"
              >
                {{ workload.kind }}
              </span>
              <HostBadge
                :agent-id="workload.agent_id"
                :hostname="workload.agent_hostname"
                :label="workload.agent_label"
                class="flex-shrink-0"
              />
              <span
                :class="[
                  'text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border flex-shrink-0',
                  statusStyle(workload.status),
                ]"
              >
                {{ workload.status }}
              </span>
            </div>

            <!-- Right: replicas + image + age -->
            <div class="flex items-center gap-4 flex-shrink-0">
              <span
                :class="[
                  'text-sm font-semibold tabular-nums',
                  replicaColor(workload.ready_replicas, workload.desired_replicas),
                ]"
              >
                {{ workload.ready_replicas }}/{{ workload.desired_replicas }}
              </span>
              <div
                v-if="workload.images.length > 0"
                class="hidden sm:flex items-center gap-1.5 max-w-48"
              >
                <span class="text-xs text-mnt-muted font-mono truncate">
                  {{ primaryImage(workload.images).name.split('/').pop() }}
                </span>
                <span
                  class="text-[10px] font-mono text-mnt-muted bg-mnt-elevated px-1 py-0.5 rounded flex-shrink-0"
                >
                  {{ primaryImage(workload.images).tag }}
                </span>
              </div>
              <span class="text-xs text-mnt-muted tabular-nums hidden md:block">
                {{ timeAgo(workload.last_transition) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
