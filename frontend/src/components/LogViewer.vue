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
import type { UseLogStreamReturn } from '@/composables/useLogStream'
import type { UseLogSearchReturn } from '@/composables/useLogSearch'
import { useLogViewState } from '@/composables/useLogViewState'
import LogToolbar from './LogToolbar.vue'
import LogNewLinesBadge from './LogNewLinesBadge.vue'
import LogLineContent from './LogLineContent.vue'

const props = defineProps<{
  logStream: UseLogStreamReturn
  isExpanded: boolean
  search: UseLogSearchReturn
}>()

const emit = defineEmits<{
  'toggle-expand': []
}>()

const { expandedJsonIds, getActiveMatchOffset, toggleJsonExpand } =
  useLogViewState(props.logStream, props.search)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <LogToolbar
      :is-expanded="isExpanded"
      :status="logStream.status.value"
      :word-wrap="logStream.wordWrap.value"
      :search="search"
      @toggle-expand="emit('toggle-expand')"
      @toggle-wrap="logStream.wordWrap.value = !logStream.wordWrap.value"
      @reconnect="logStream.connect()"
    />

    <!-- Loading -->
    <div
      v-if="logStream.status.value === 'connecting' && logStream.lines.value.length === 0"
      class="flex justify-center rounded-b-xl border border-t-0 border-slate-800 bg-[#0B0E13] py-4"
    >
      <div class="h-5 w-5 animate-spin rounded-full border-2 border-slate-700 border-t-pb-green-400" />
    </div>

    <!-- Error (no lines) -->
    <div
      v-else-if="logStream.error.value && logStream.lines.value.length === 0"
      class="rounded-b-xl border border-t-0 border-slate-800 bg-[#0B0E13] p-3 text-sm text-red-400"
    >
      {{ logStream.error.value }}
    </div>

    <!-- No logs -->
    <div
      v-else-if="logStream.lines.value.length === 0"
      class="rounded-b-xl border border-t-0 border-slate-800 bg-[#0B0E13] py-4 text-center text-sm text-slate-500"
    >
      No logs available
    </div>

    <!-- Log content -->
    <div v-else class="relative min-h-0 flex-1">
      <div
        :ref="(el: any) => { logStream.scrollContainerRef.value = el }"
        class="absolute inset-0 overflow-auto rounded-b-xl border border-t-0 border-slate-800 bg-[#0B0E13] px-2 py-1 font-mono text-[0.7rem] leading-relaxed text-white"
        :class="logStream.wordWrap.value ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'"
        @scroll="logStream.handleScroll"
      >
        <LogLineContent
          v-for="(line, idx) in logStream.lines.value"
          :key="line.id"
          :data-line-index="idx"
          :line="line"
          :line-index="idx"
          :has-timestamps="logStream.hasTimestamps.value"
          :search-matches="search.getLineMatches(idx)"
          :active-match-offset="getActiveMatchOffset(idx)"
          :expanded="expandedJsonIds.has(line.id)"
          @toggle-expand="toggleJsonExpand(line.id)"
        />
      </div>

      <LogNewLinesBadge
        :unseen-count="logStream.unseenCount.value"
        @click="logStream.scrollToBottom()"
      />
    </div>

    <!-- Error banner (with lines still visible) -->
    <div
      v-if="logStream.error.value && logStream.lines.value.length > 0"
      class="mt-2 rounded-lg border border-slate-800 bg-[#12151C] px-3 py-2 text-xs text-slate-400"
    >
      {{ logStream.error.value }}
    </div>
  </div>
</template>
