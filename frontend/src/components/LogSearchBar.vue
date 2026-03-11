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
import { ref, computed, watch, nextTick } from 'vue'
import { ChevronUp, ChevronDown, X } from 'lucide-vue-next'
import type { UseLogSearchReturn } from '@/composables/useLogSearch'

const props = defineProps<{
  search: UseLogSearchReturn
}>()

const inputRef = ref<HTMLInputElement | null>(null)

watch(() => props.search.isOpen.value, (open) => {
  if (open) {
    nextTick(() => inputRef.value?.focus())
  }
})

function onInput(e: Event) {
  props.search.setQuery((e.target as HTMLInputElement).value)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && e.shiftKey) {
    e.preventDefault()
    props.search.prevMatch()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    props.search.nextMatch()
  } else if (e.key === 'Escape') {
    e.preventDefault()
    props.search.close()
  }
}

const matchDisplay = computed(() => {
  const total = props.search.matches.value.length
  const idx = props.search.currentMatchIndex.value
  if (!props.search.query.value) return ''
  if (total === 0) return 'No results'
  return `${idx + 1}/${total}`
})
</script>

<template>
  <div
    v-if="search.isOpen.value"
    class="flex items-center gap-1.5 rounded-lg border bg-[#0B0E13] px-2 py-1"
    :class="search.isValid.value ? 'border-slate-800' : 'border-red-500'"
  >
    <input
      ref="inputRef"
      type="text"
      :value="search.query.value"
      placeholder="Search logs..."
      class="w-32 bg-transparent text-xs text-white placeholder-slate-600 outline-none sm:w-48"
      @input="onInput"
      @keydown="onKeydown"
    />

    <!-- Match counter -->
    <span
      v-if="search.query.value"
      class="shrink-0 text-[10px] tabular-nums"
      :class="search.matches.value.length > 0 ? 'text-slate-400' : 'text-slate-600'"
    >{{ matchDisplay }}</span>

    <!-- Case sensitive toggle -->
    <button
      class="shrink-0 rounded px-1 py-0.5 text-[10px] font-bold transition-colors"
      :class="search.isCaseSensitive.value
        ? 'bg-slate-700 text-slate-200'
        : 'text-slate-500 hover:text-slate-300'"
      title="Match Case"
      @click="search.toggleCaseSensitive()"
    >Aa</button>

    <!-- Regex toggle -->
    <button
      class="shrink-0 rounded px-1 py-0.5 text-[10px] font-bold transition-colors"
      :class="search.isRegex.value
        ? 'bg-slate-700 text-slate-200'
        : 'text-slate-500 hover:text-slate-300'"
      title="Use Regular Expression"
      @click="search.toggleRegex()"
    >.*</button>

    <!-- Navigation -->
    <button
      class="shrink-0 rounded p-0.5 text-slate-500 transition-colors hover:text-slate-300"
      title="Previous Match (Shift+Enter)"
      :disabled="search.matches.value.length === 0"
      @click="search.prevMatch()"
    >
      <ChevronUp :size="12" />
    </button>
    <button
      class="shrink-0 rounded p-0.5 text-slate-500 transition-colors hover:text-slate-300"
      title="Next Match (Enter)"
      :disabled="search.matches.value.length === 0"
      @click="search.nextMatch()"
    >
      <ChevronDown :size="12" />
    </button>

    <!-- Close -->
    <button
      class="shrink-0 rounded p-0.5 text-slate-500 transition-colors hover:text-slate-300"
      title="Close (Escape)"
      @click="search.close()"
    >
      <X :size="12" />
    </button>
  </div>
</template>
