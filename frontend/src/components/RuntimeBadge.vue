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
import { Box, Layers, Cloud } from 'lucide-vue-next'
import { useRuntime } from '@/composables/useRuntime'
import { useRuntimeStore } from '@/stores/runtime'

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
  <div class="relative" @mouseenter="onEnter" @mouseleave="onLeave">
    <button
      class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium transition-all hover:bg-slate-800/60 border border-transparent hover:border-slate-700/50 cursor-pointer"
      @click="onClick"
    >
      <!-- Status dot -->
      <span
        class="inline-block h-2 w-2 rounded-full shrink-0"
        :style="{ backgroundColor: connected ? '#10b981' : '#f43f5e' }"
      />

      <!-- Runtime icon -->
      <Layers v-if="isSwarm" :size="16" class="text-slate-400 shrink-0" />
      <Cloud v-else-if="isKubernetes" :size="16" class="text-slate-400 shrink-0" />
      <Box v-else :size="16" class="text-slate-400 shrink-0" />

      <!-- Label -->
      <span class="text-pb-secondary">{{ capitalize(runtimeContext) }}</span>
    </button>

    <!-- Popover -->
    <Transition
      enter-active-class="transition duration-100 ease-out"
      enter-from-class="opacity-0 scale-95 -translate-y-1"
      enter-to-class="opacity-100 scale-100 translate-y-0"
      leave-active-class="transition duration-75 ease-in"
      leave-from-class="opacity-100 scale-100 translate-y-0"
      leave-to-class="opacity-0 scale-95 -translate-y-1"
    >
      <div
        v-if="open"
        class="absolute right-0 top-full mt-2 w-64 rounded-xl border border-slate-700 bg-pb-surface shadow-2xl shadow-black/40 overflow-hidden z-50"
        @mouseenter="onEnter"
        @mouseleave="onLeave"
      >
        <!-- Header -->
        <div class="px-4 py-3 border-b border-slate-800">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Runtime Context</span>
        </div>

        <!-- Body -->
        <div class="px-4 py-3 space-y-2">
          <!-- Runtime -->
          <div class="flex justify-between items-center">
            <span class="text-xs text-slate-500">Runtime</span>
            <span class="text-sm text-pb-primary">{{ capitalize(store.runtime) }}</span>
          </div>

          <!-- Context -->
          <div class="flex justify-between items-center">
            <span class="text-xs text-slate-500">Context</span>
            <span class="text-sm text-pb-primary">{{ capitalize(runtimeContext) }}</span>
          </div>

          <!-- Status -->
          <div class="flex justify-between items-center">
            <span class="text-xs text-slate-500">Status</span>
            <span
              class="text-sm font-medium"
              :class="connected ? 'text-pb-status-ok' : 'text-pb-status-down'"
            >{{ connected ? 'Connected' : 'Disconnected' }}</span>
          </div>

          <!-- Detected at -->
          <div class="flex justify-between items-center">
            <span class="text-xs text-slate-500">Detected at</span>
            <span class="text-sm text-slate-400">{{ formatDetectedAt(store.detectedAt) }}</span>
          </div>

          <!-- Swarm metadata -->
          <template v-if="isSwarm && 'cluster_id' in store.metadata">
            <div class="pt-1 mt-1 border-t border-slate-800/60 space-y-2">
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">Cluster ID</span>
                <span class="text-sm text-pb-primary font-mono">{{ (store.metadata as { cluster_id: string }).cluster_id.slice(0, 12) }}</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">Managers</span>
                <span class="text-sm text-pb-primary">{{ (store.metadata as { manager_count: number }).manager_count }}</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">Workers</span>
                <span class="text-sm text-pb-primary">{{ (store.metadata as { worker_count: number }).worker_count }}</span>
              </div>
            </div>
          </template>

          <!-- Kubernetes metadata -->
          <template v-if="isKubernetes && 'namespace_count' in store.metadata">
            <div class="pt-1 mt-1 border-t border-slate-800/60 space-y-2">
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">Namespaces</span>
                <span class="text-sm text-pb-primary">{{ (store.metadata as { namespace_count: number }).namespace_count }}</span>
              </div>
              <div class="flex justify-between items-center">
                <span class="text-xs text-slate-500">Nodes</span>
                <span class="text-sm text-pb-primary">{{ (store.metadata as { node_count: number }).node_count }}</span>
              </div>
            </div>
          </template>
        </div>
      </div>
    </Transition>
  </div>
</template>
