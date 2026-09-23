<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { Maximize2, Minimize2, WrapText, Search } from 'lucide-vue-next'
import type { LogStreamStatus } from '@/composables/useLogStream'
import type { UseLogSearchReturn } from '@/composables/useLogSearch'
import LogSearchBar from './LogSearchBar.vue'
import UiButton from '@/components/ui/UiButton.vue'

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
    class="flex items-center justify-between rounded-t-xl border border-b-0 border-mnt-default bg-mnt-surface px-3 py-2"
  >
    <div class="flex items-center gap-2">
      <h3 class="text-xs font-semibold text-mnt-muted">Logs</h3>
      <span
        v-if="status === 'streaming'"
        class="flex items-center gap-1 text-xs text-mnt-green-400"
      >
        <span class="inline-block h-1.5 w-1.5 rounded-full bg-mnt-green-400" />
        Streaming
      </span>
      <UiButton
        v-if="status === 'closed' || status === 'error'"
        variant="ghost"
        size="sm"
        @click="emit('reconnect')"
      >
        Reconnect
      </UiButton>
    </div>

    <div class="flex items-center gap-1">
      <!-- Search bar (inline when open) -->
      <LogSearchBar :search="search" />

      <!-- Search toggle button (when search closed) -->
      <UiButton
        v-if="!search.isOpen.value"
        variant="ghost"
        size="sm"
        :icon="Search"
        title="Search (Ctrl+K)"
        aria-label="Search logs"
        @click="search.open()"
      />

      <UiButton
        variant="ghost"
        size="sm"
        :icon="WrapText"
        :class="{ 'text-mnt-primary bg-mnt-elevated': !wordWrap }"
        :title="wordWrap ? 'Disable word wrap' : 'Enable word wrap'"
        :aria-label="wordWrap ? 'Disable word wrap' : 'Enable word wrap'"
        @click="emit('toggle-wrap')"
      />
      <UiButton
        variant="ghost"
        size="sm"
        :title="isExpanded ? 'Collapse' : 'Expand'"
        :aria-label="isExpanded ? 'Collapse log viewer' : 'Expand log viewer'"
        @click="emit('toggle-expand')"
      >
        <Maximize2 v-if="!isExpanded" :size="14" />
        <Minimize2 v-else :size="14" />
      </UiButton>
    </div>
  </div>
</template>
