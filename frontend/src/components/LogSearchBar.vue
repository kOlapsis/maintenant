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
import UiButton from '@/components/ui/UiButton.vue'
import TextInput from '@/components/ui/TextInput.vue'

const props = defineProps<{
  search: UseLogSearchReturn
}>()

const inputRef = ref<InstanceType<typeof TextInput> | null>(null)

watch(() => props.search.isOpen.value, (open) => {
  if (open) {
    nextTick(() => inputRef.value?.focus())
  }
})

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
    <TextInput
      ref="inputRef"
      variant="bare"
      :model-value="search.query.value"
      placeholder="Search logs..."
      class="w-32 sm:w-48"
      @update:model-value="(v) => search.setQuery(String(v ?? ''))"
      @keydown="onKeydown"
    />

    <!-- Match counter -->
    <span
      v-if="search.query.value"
      class="shrink-0 text-[10px] tabular-nums"
      :class="search.matches.value.length > 0 ? 'text-mnt-muted' : 'text-mnt-muted'"
    >{{ matchDisplay }}</span>

    <!-- Case sensitive toggle -->
    <UiButton
      variant="ghost"
      size="sm"
      class="shrink-0"
      :class="search.isCaseSensitive.value ? 'bg-mnt-elevated text-mnt-primary' : ''"
      title="Match Case"
      aria-label="Match case"
      :aria-pressed="search.isCaseSensitive.value"
      @click="search.toggleCaseSensitive()"
    >Aa</UiButton>

    <!-- Regex toggle -->
    <UiButton
      variant="ghost"
      size="sm"
      class="shrink-0"
      :class="search.isRegex.value ? 'bg-mnt-elevated text-mnt-primary' : ''"
      title="Use Regular Expression"
      aria-label="Use regular expression"
      :aria-pressed="search.isRegex.value"
      @click="search.toggleRegex()"
    >.*</UiButton>

    <!-- Navigation -->
    <UiButton
      variant="ghost"
      size="sm"
      class="shrink-0"
      :icon="ChevronUp"
      title="Previous Match (Shift+Enter)"
      aria-label="Previous match"
      :disabled="search.matches.value.length === 0"
      @click="search.prevMatch()"
    />
    <UiButton
      variant="ghost"
      size="sm"
      class="shrink-0"
      :icon="ChevronDown"
      title="Next Match (Enter)"
      aria-label="Next match"
      :disabled="search.matches.value.length === 0"
      @click="search.nextMatch()"
    />

    <!-- Close -->
    <UiButton
      variant="ghost"
      size="sm"
      class="shrink-0"
      :icon="X"
      title="Close (Escape)"
      aria-label="Close search"
      @click="search.close()"
    />
  </div>
</template>
