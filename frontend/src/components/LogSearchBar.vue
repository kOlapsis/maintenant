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
    class="flex items-center gap-1.5 rounded-lg border bg-mnt-primary px-2 py-1"
    :class="search.isValid.value ? 'border-mnt-default' : 'border-red-500'"
  >
    <input
      ref="inputRef"
      type="text"
      :value="search.query.value"
      placeholder="Search logs..."
      class="w-32 bg-transparent text-xs text-mnt-primary placeholder:text-mnt-muted outline-none sm:w-48"
      @input="onInput"
      @keydown="onKeydown"
    />

    <!-- Match counter -->
    <span
      v-if="search.query.value"
      class="shrink-0 text-[10px] tabular-nums"
      :class="search.matches.value.length > 0 ? 'text-mnt-muted' : 'text-mnt-muted'"
    >{{ matchDisplay }}</span>

    <!-- Case sensitive toggle -->
    <button
      class="shrink-0 rounded px-1 py-0.5 text-[10px] font-bold transition-colors"
      :class="search.isCaseSensitive.value
        ? 'bg-mnt-elevated text-mnt-primary'
        : 'text-mnt-muted hover:text-mnt-secondary'"
      title="Match Case"
      @click="search.toggleCaseSensitive()"
    >Aa</button>

    <!-- Regex toggle -->
    <button
      class="shrink-0 rounded px-1 py-0.5 text-[10px] font-bold transition-colors"
      :class="search.isRegex.value
        ? 'bg-mnt-elevated text-mnt-primary'
        : 'text-mnt-muted hover:text-mnt-secondary'"
      title="Use Regular Expression"
      @click="search.toggleRegex()"
    >.*</button>

    <!-- Navigation -->
    <button
      class="shrink-0 rounded p-0.5 text-mnt-muted transition-colors hover:text-mnt-secondary"
      title="Previous Match (Shift+Enter)"
      aria-label="Previous match"
      :disabled="search.matches.value.length === 0"
      @click="search.prevMatch()"
    >
      <ChevronUp :size="12" />
    </button>
    <button
      class="shrink-0 rounded p-0.5 text-mnt-muted transition-colors hover:text-mnt-secondary"
      title="Next Match (Enter)"
      aria-label="Next match"
      :disabled="search.matches.value.length === 0"
      @click="search.nextMatch()"
    >
      <ChevronDown :size="12" />
    </button>

    <!-- Close -->
    <button
      class="shrink-0 rounded p-0.5 text-mnt-muted transition-colors hover:text-mnt-secondary"
      title="Close (Escape)"
      aria-label="Close search"
      @click="search.close()"
    >
      <X :size="12" />
    </button>
  </div>
</template>
