<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Search, X } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    /** Number of items left after filtering, shown once a query is typed. */
    resultCount?: number
  }>(),
  { placeholder: 'Search', resultCount: undefined },
)

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const input = ref<HTMLInputElement | null>(null)

// "/" focuses the field the way it does in a file manager or a code host,
// but never while the operator is already typing somewhere else.
function onKeydown(e: KeyboardEvent) {
  if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return
  const el = document.activeElement
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return
  if (el instanceof HTMLElement && el.isContentEditable) return
  e.preventDefault()
  input.value?.focus()
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

function clear() {
  emit('update:modelValue', '')
  input.value?.focus()
}
</script>

<template>
  <div class="search-field relative flex items-center">
    <Search
      :size="14"
      class="pointer-events-none absolute left-3 text-mnt-muted"
      aria-hidden="true"
    />
    <input
      ref="input"
      type="search"
      role="searchbox"
      :value="modelValue"
      :placeholder="placeholder"
      :aria-label="placeholder"
      class="focus-ring w-full rounded-lg border border-mnt-default bg-mnt-elevated py-2 pl-9 pr-16 text-sm text-mnt-primary"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
      @keydown.esc="clear"
    />
    <span
      v-if="modelValue && resultCount !== undefined"
      class="pointer-events-none absolute right-9 text-xs tabular-nums text-mnt-muted"
    >
      {{ resultCount }}
    </span>
    <button
      v-if="modelValue"
      type="button"
      class="focus-ring absolute right-2 rounded p-1 text-mnt-muted hover:text-mnt-primary"
      aria-label="Clear search"
      @click="clear"
    >
      <X :size="14" aria-hidden="true" />
    </button>
  </div>
</template>

<style scoped>
.search-field input::placeholder {
  color: var(--mnt-text-muted);
}
/* The native clear affordance duplicates the button and ignores the theme. */
.search-field input::-webkit-search-cancel-button {
  display: none;
}
</style>
