<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license.
-->
<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useKubernetesStore } from '@/stores/kubernetes'
import { timeAgo } from '@/utils/time'

const emit = defineEmits<{
  select: [name: string]
}>()

const k8sStore = useKubernetesStore()

onMounted(() => {
  k8sStore.fetchNodesList()
  k8sStore.startListening()
})

onUnmounted(() => {
  k8sStore.stopListening()
})

const sortedNodes = computed(() => {
  return [...k8sStore.nodes].sort((a, b) => {
    // Control plane first, then alphabetical.
    const aCP = a.roles.includes('control-plane') || a.roles.includes('master')
    const bCP = b.roles.includes('control-plane') || b.roles.includes('master')
    if (aCP !== bCP) return aCP ? -1 : 1
    return a.name.localeCompare(b.name)
  })
})

const readyCount = computed(() => k8sStore.nodes.filter((n) => n.status === 'ready').length)

function statusVar(status: string): string {
  switch (status) {
    case 'ready': return 'var(--mnt-sev-ok)'
    case 'not-ready': return 'var(--mnt-sev-incident)'
    default: return 'var(--mnt-sev-neutral)'
  }
}

function statusText(status: string): string {
  switch (status) {
    case 'ready': return 'text-mnt-status-ok'
    case 'not-ready': return 'text-mnt-status-down'
    default: return 'text-mnt-muted'
  }
}

function roleStyle(_role: string): string {
  return 'text-mnt-secondary bg-mnt-elevated border-mnt-default'
}

function hasCondition(conditions: Array<{ type: string; status: string }>, condType: string): boolean {
  return conditions.some((c) => c.type === condType && c.status === 'True')
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 1 ? 1 : 0)} ${units[i]}`
}

function formatCPU(millicores: number): string {
  if (millicores >= 1000) return `${(millicores / 1000).toFixed(1)} cores`
  return `${millicores}m`
}
</script>

<template>
  <div>
    <div class="mb-3 flex items-center justify-between">
      <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Cluster Nodes</p>
      <div class="flex items-center gap-3 text-xs text-mnt-muted">
        <span :class="readyCount === k8sStore.nodes.length ? 'text-mnt-status-ok' : 'text-mnt-status-warn'">
          {{ readyCount }}/{{ k8sStore.nodes.length }} ready
        </span>
      </div>
    </div>

    <div v-if="k8sStore.nodesLoading && k8sStore.nodes.length === 0" class="text-sm text-mnt-muted py-8 text-center">
      Loading nodes...
    </div>

    <div v-else-if="k8sStore.error && k8sStore.nodes.length === 0" class="text-sm text-mnt-status-down py-8 text-center">
      {{ k8sStore.error }}
    </div>

    <div v-else-if="k8sStore.nodes.length === 0" class="text-sm text-mnt-muted py-8 text-center">
      No nodes found
    </div>

    <div v-else class="space-y-1">
      <div
        v-for="node in sortedNodes"
        :key="node.name"
        class="bg-mnt-surface rounded-xl border border-mnt-default px-4 py-3 hover:bg-mnt-elevated transition-all cursor-pointer group"
        @click="emit('select', node.name)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3 min-w-0">
            <!-- Status dot -->
            <div class="relative flex-shrink-0">
              <div class="w-2.5 h-2.5 rounded-full" :style="{ backgroundColor: node.stale ? 'var(--mnt-sev-unknown)' : statusVar(node.status) }" />
              <div
                v-if="node.status === 'ready' && !node.stale"
                class="mnt-ping absolute inset-0 w-2.5 h-2.5 rounded-full opacity-30"
                :style="{ backgroundColor: statusVar(node.status) }"
              />
            </div>

            <!-- Node name -->
            <span class="text-sm text-mnt-primary font-medium truncate">{{ node.name }}</span>

            <!-- Role badges -->
            <span
              v-for="role in node.roles"
              :key="role"
              :class="['text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border', roleStyle(role)]"
            >
              {{ role }}
            </span>

            <!-- Condition warnings -->
            <span
              v-if="hasCondition(node.conditions, 'MemoryPressure')"
              class="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning"
            >
              MemPressure
            </span>
            <span
              v-if="hasCondition(node.conditions, 'DiskPressure')"
              class="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning"
            >
              DiskPressure
            </span>
            <span
              v-if="hasCondition(node.conditions, 'PIDPressure')"
              class="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded border text-mnt-status-warn bg-mnt-status-warn border-mnt-sev-warning"
            >
              PIDPressure
            </span>
          </div>

          <div class="flex items-center gap-4 text-xs text-mnt-muted flex-shrink-0 ml-4">
            <!-- Status text -->
            <span
              :class="['font-medium', node.stale ? 'text-mnt-sev-unknown' : statusText(node.status)]"
              :title="node.stale ? `Agent offline · last known: ${node.status}` : undefined"
            >{{ node.stale ? 'offline' : node.status }}</span>

            <!-- Capacity summary -->
            <span class="tabular-nums hidden sm:inline" :title="`${node.capacity.cpu_millicores}m CPU`">
              {{ formatCPU(node.capacity.cpu_millicores) }}
            </span>
            <span class="tabular-nums hidden sm:inline" :title="`${node.capacity.memory_bytes} bytes`">
              {{ formatBytes(node.capacity.memory_bytes) }}
            </span>

            <!-- Running pods -->
            <span class="tabular-nums">{{ node.running_pods }} pods</span>

            <!-- K8s version -->
            <span v-if="node.kubernetes_version" class="text-mnt-muted hidden md:inline">
              {{ node.kubernetes_version }}
            </span>

            <!-- Created -->
            <span class="text-mnt-muted tabular-nums hidden lg:inline" :title="node.created_at">
              {{ timeAgo(node.created_at) }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
