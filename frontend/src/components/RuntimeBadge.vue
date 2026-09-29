<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref } from 'vue'
import { Box, Layers, Cloud } from 'lucide-vue-next'
import { useRuntime } from '@/composables/useRuntime'
import { useRuntimeStore } from '@/stores/runtime'
import PopoverMenu from '@/components/ui/PopoverMenu.vue'
import UiButton from '@/components/ui/UiButton.vue'

const { runtimeContext, connected, isSwarm, isKubernetes } = useRuntime()
const store = useRuntimeStore()

const open = ref(false)
let closeTimeout: ReturnType<typeof setTimeout> | null = null

function onEnter() {
  if (closeTimeout) { clearTimeout(closeTimeout); closeTimeout = null }
  open.value = true
}

function onLeave() {
  closeTimeout = setTimeout(() => { open.value = false }, 150)
}

function onClick() {
  open.value = !open.value
}

function capitalize(s: string): string {
  if (!s) return s
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function formatDetectedAt(iso: string | null): string {
  if (!iso) return '—'
  try {
    const date = new Date(iso)
    const now = Date.now()
    const diffMs = now - date.getTime()
    const diffMin = Math.floor(diffMs / 60_000)
    if (diffMin < 1) return 'just now'
    if (diffMin < 60) return `${diffMin}m ago`
    const diffH = Math.floor(diffMin / 60)
    if (diffH < 24) return `${diffH}h ago`
    const diffD = Math.floor(diffH / 24)
    return `${diffD}d ago`
  } catch {
    return iso
  }
}
</script>

<template>
  <PopoverMenu
    v-model:open="open"
    ariaLabel="Runtime context"
    panel-role="group"
    panel-class="w-64"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
  >
    <template #trigger>
      <UiButton
        variant="ghost"
        class="border border-transparent text-xs hover:border-mnt-default/50"
        @click="onClick"
      >
        <!-- Status dot -->
        <span
          class="inline-block h-2 w-2 rounded-full shrink-0"
          :style="{ backgroundColor: connected ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)' }"
        />

        <!-- Runtime icon -->
        <Layers v-if="isSwarm" :size="16" class="text-mnt-muted shrink-0" />
        <Cloud v-else-if="isKubernetes" :size="16" class="text-mnt-muted shrink-0" />
        <Box v-else :size="16" class="text-mnt-muted shrink-0" />

        <!-- Label -->
        <span class="text-mnt-secondary">{{ capitalize(runtimeContext) }}</span>
      </UiButton>
    </template>

    <!-- Header -->
    <div class="px-4 py-3 border-b border-mnt-default">
      <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">Runtime Context</span>
    </div>

    <!-- Body -->
    <div class="px-4 py-3 space-y-2">
      <!-- Runtime -->
      <div class="flex justify-between items-center">
        <span class="text-xs text-mnt-muted">Runtime</span>
        <span class="text-sm text-mnt-primary">{{ capitalize(store.runtime) }}</span>
      </div>

      <!-- Context -->
      <div class="flex justify-between items-center">
        <span class="text-xs text-mnt-muted">Context</span>
        <span class="text-sm text-mnt-primary">{{ capitalize(runtimeContext) }}</span>
      </div>

      <!-- Status -->
      <div class="flex justify-between items-center">
        <span class="text-xs text-mnt-muted">Status</span>
        <span
          class="text-sm font-medium"
          :class="connected ? 'text-mnt-status-ok' : 'text-mnt-status-down'"
        >{{ connected ? 'Connected' : 'Disconnected' }}</span>
      </div>

      <!-- Detected at -->
      <div class="flex justify-between items-center">
        <span class="text-xs text-mnt-muted">Detected at</span>
        <span class="text-sm text-mnt-muted">{{ formatDetectedAt(store.detectedAt) }}</span>
      </div>

      <!-- Swarm metadata -->
      <template v-if="isSwarm && 'cluster_id' in store.metadata">
        <div class="pt-1 mt-1 border-t border-mnt-default/60 space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-xs text-mnt-muted">Cluster ID</span>
            <span class="text-sm text-mnt-primary font-mono">{{ (store.metadata as { cluster_id: string }).cluster_id.slice(0, 12) }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-xs text-mnt-muted">Managers</span>
            <span class="text-sm text-mnt-primary">{{ (store.metadata as { manager_count: number }).manager_count }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-xs text-mnt-muted">Workers</span>
            <span class="text-sm text-mnt-primary">{{ (store.metadata as { worker_count: number }).worker_count }}</span>
          </div>
        </div>
      </template>

      <!-- Kubernetes metadata -->
      <template v-if="isKubernetes && 'namespace_count' in store.metadata">
        <div class="pt-1 mt-1 border-t border-mnt-default/60 space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-xs text-mnt-muted">Namespaces</span>
            <span class="text-sm text-mnt-primary">{{ (store.metadata as { namespace_count: number }).namespace_count }}</span>
          </div>
          <div class="flex justify-between items-center">
            <span class="text-xs text-mnt-muted">Nodes</span>
            <span class="text-sm text-mnt-primary">{{ (store.metadata as { node_count: number }).node_count }}</span>
          </div>
        </div>
      </template>
    </div>
  </PopoverMenu>
</template>
