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
import { onMounted, onUnmounted } from 'vue'
import type { UseLogStreamReturn } from '@/composables/useLogStream'
import type { UseLogSearchReturn } from '@/composables/useLogSearch'
import { useLogViewState } from '@/composables/useLogViewState'
import LogToolbar from './LogToolbar.vue'
import LogNewLinesBadge from './LogNewLinesBadge.vue'
import LogLineContent from './LogLineContent.vue'

const props = defineProps<{
  containerName: string
  logStream: UseLogStreamReturn
  search: UseLogSearchReturn
}>()

const emit = defineEmits<{
  close: []
}>()

const { expandedJsonIds, getActiveMatchOffset, toggleJsonExpand } =
  useLogViewState(props.logStream, props.search)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && !props.search.isOpen.value) {
    e.stopImmediatePropagation()
    emit('close')
  }
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown, true)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown, true)
})
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-[10000] flex flex-col bg-pb-primary">
      <!-- Header -->
      <div class="flex items-center gap-3 border-b border-slate-800 px-4 py-2">
        <span class="text-sm font-semibold text-pb-primary">{{ containerName }}</span>
        <span class="flex-1" />
        <LogToolbar
          :is-expanded="true"
          :status="logStream.status.value"
          :word-wrap="logStream.wordWrap.value"
          :search="search"
          @toggle-expand="emit('close')"
          @toggle-wrap="logStream.wordWrap.value = !logStream.wordWrap.value"
          @reconnect="logStream.connect()"
        />
      </div>

      <!-- Log content -->
      <div class="relative flex-1">
        <div
          :ref="(el: any) => { logStream.scrollContainerRef.value = el }"
          class="absolute inset-0 overflow-auto px-2 py-1 font-mono text-[0.7rem] leading-relaxed text-pb-primary"
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
    </div>
  </Teleport>
</template>
