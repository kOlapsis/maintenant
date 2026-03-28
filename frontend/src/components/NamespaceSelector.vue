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
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useNamespacesStore } from '@/stores/namespaces'
import { ChevronDown, Check, Layers } from 'lucide-vue-next'

const store = useNamespacesStore()

const open = ref(false)
const containerRef = ref<HTMLElement | null>(null)

const label = computed(() => {
  const count = store.selectedNamespaces.length
  if (count === 0) return 'All namespaces'
  if (count === 1) return store.selectedNamespaces[0] ?? 'All namespaces'
  return `${count} namespaces`
})

function toggle() {
  open.value = !open.value
}

function handleClickOutside(e: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(e.target as Node)) {
    open.value = false
  }
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
  store.fetchNamespacesList()
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleClickOutside)
})
</script>

<template>
  <div ref="containerRef" class="relative">
    <button
      class="flex items-center gap-2 bg-[#12151C] border border-slate-800 rounded-lg px-3 py-2 text-sm text-slate-300 hover:border-slate-700 hover:text-white transition-all"
      @click="toggle"
    >
      <Layers :size="14" class="text-slate-500 flex-shrink-0" />
      <span class="truncate max-w-40">{{ label }}</span>
      <ChevronDown
        :size="14"
        :class="['text-slate-500 flex-shrink-0 transition-transform', open ? 'rotate-180' : '']"
      />
    </button>

    <!-- Dropdown -->
    <div
      v-if="open"
      class="absolute right-0 top-full mt-1 z-50 min-w-52 bg-[#12151C] border border-slate-800 rounded-xl shadow-xl overflow-hidden"
    >
      <!-- All namespaces option -->
      <button
        class="w-full flex items-center justify-between px-4 py-2.5 text-sm text-slate-300 hover:bg-slate-800/40 hover:text-white transition-colors"
        @click="store.selectAll()"
      >
        <span>All namespaces</span>
        <Check
          v-if="store.selectedNamespaces.length === 0"
          :size="14"
          class="text-pb-green-400 flex-shrink-0"
        />
      </button>

      <!-- Divider -->
      <div v-if="store.namespaces.length > 0" class="border-t border-slate-800" />

      <!-- Individual namespaces -->
      <div class="max-h-64 overflow-y-auto">
        <button
          v-for="ns in store.namespaces"
          :key="ns"
          class="w-full flex items-center justify-between px-4 py-2.5 text-sm hover:bg-slate-800/40 transition-colors"
          :class="store.selectedNamespaces.includes(ns) ? 'text-white' : 'text-slate-400'"
          @click="store.toggleNamespace(ns)"
        >
          <span class="font-mono">{{ ns }}</span>
          <Check
            v-if="store.selectedNamespaces.includes(ns)"
            :size="14"
            class="text-pb-green-400 flex-shrink-0"
          />
        </button>
      </div>

      <!-- Empty state -->
      <div v-if="store.namespaces.length === 0" class="px-4 py-3 text-xs text-slate-500 text-center">
        No namespaces found
      </div>
    </div>
  </div>
</template>
