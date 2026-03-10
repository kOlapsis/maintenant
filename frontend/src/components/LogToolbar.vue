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
import { Maximize2, Minimize2, WrapText, Search } from 'lucide-vue-next'
import type { LogStreamStatus } from '@/composables/useLogStream'
import type { UseLogSearchReturn } from '@/composables/useLogSearch'
import LogSearchBar from './LogSearchBar.vue'

defineProps<{
  isExpanded: boolean
  status: LogStreamStatus
  wordWrap: boolean
  search: UseLogSearchReturn
}>()

const emit = defineEmits<{
  'toggle-expand': []
  'toggle-wrap': []
  reconnect: []
}>()
</script>

<template>
  <div
    class="flex items-center justify-between rounded-t-xl border border-b-0 border-slate-800 bg-[#12151C] px-3 py-2"
  >
    <div class="flex items-center gap-2">
      <h3 class="text-xs font-semibold text-slate-400">Logs</h3>
      <span
        v-if="status === 'streaming'"
        class="flex items-center gap-1 text-xs text-pb-green-400"
      >
        <span class="inline-block h-1.5 w-1.5 rounded-full bg-pb-green-400" />
        Streaming
      </span>
      <button
        v-if="status === 'closed' || status === 'error'"
        class="rounded px-2 py-0.5 text-xs text-slate-400 transition-colors hover:bg-slate-800 hover:text-slate-200"
        @click="emit('reconnect')"
      >
        Reconnect
      </button>
    </div>

    <div class="flex items-center gap-1">
      <!-- Search bar (inline when open) -->
      <LogSearchBar :search="search" />

      <!-- Search toggle button (when search closed) -->
      <button
        v-if="!search.isOpen.value"
        class="rounded p-1.5 text-slate-500 transition-colors hover:bg-slate-800 hover:text-slate-200"
        title="Search (Ctrl+K)"
        @click="search.open()"
      >
        <Search :size="14" />
      </button>

      <button
        class="rounded p-1.5 text-slate-500 transition-colors hover:bg-slate-800 hover:text-slate-200"
        :class="{ 'text-slate-200 bg-slate-800': !wordWrap }"
        :title="wordWrap ? 'Disable word wrap' : 'Enable word wrap'"
        @click="emit('toggle-wrap')"
      >
        <WrapText :size="14" />
      </button>
      <button
        class="rounded p-1.5 text-slate-500 transition-colors hover:bg-slate-800 hover:text-slate-200"
        :title="isExpanded ? 'Collapse' : 'Expand'"
        @click="emit('toggle-expand')"
      >
        <Maximize2 v-if="!isExpanded" :size="14" />
        <Minimize2 v-else :size="14" />
      </button>
    </div>
  </div>
</template>
