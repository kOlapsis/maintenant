<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts" generic="T extends string | number">
import { ref } from 'vue'
import CountBadge, { type CountTone } from './CountBadge.vue'

export interface TabNavItem<V extends string | number = string> {
  value: V
  label: string
  count?: number | string
  countTone?: Exclude<CountTone, 'ok'>
}

const props = defineProps<{
  items: TabNavItem<T>[]
  ariaLabel: string
}>()

const model = defineModel<T>({ required: true })

const tabRefs = ref<(HTMLButtonElement | null)[]>([])

function focusTab(index: number) {
  const item = props.items[index]
  if (!item) return
  model.value = item.value
  tabRefs.value[index]?.focus()
}

function onKeydown(event: KeyboardEvent, index: number) {
  const lastIndex = props.items.length - 1
  if (event.key === 'ArrowRight') focusTab(index === lastIndex ? 0 : index + 1)
  else if (event.key === 'ArrowLeft') focusTab(index === 0 ? lastIndex : index - 1)
  else if (event.key === 'Home') focusTab(0)
  else if (event.key === 'End') focusTab(lastIndex)
  else return
  event.preventDefault()
}
</script>

<template>
  <div class="border-b border-mnt-default">
    <div role="tablist" :aria-label="ariaLabel" class="-mb-px flex gap-6 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
      <button
        v-for="(item, index) in items"
        :key="item.value"
        :ref="(el) => (tabRefs[index] = el as HTMLButtonElement | null)"
        type="button"
        role="tab"
        :aria-selected="model === item.value"
        :tabindex="model === item.value ? 0 : -1"
        class="focus-ring inline-flex min-h-[44px] shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 pb-2 text-sm font-medium transition-colors"
        :class="
          model === item.value
            ? 'border-mnt-accent text-mnt-accent'
            : 'border-transparent text-mnt-muted hover:text-mnt-primary'
        "
        @click="focusTab(index)"
        @keydown="onKeydown($event, index)"
      >
        {{ item.label }}
        <CountBadge v-if="item.count !== undefined" :value="item.count" :tone="item.countTone ?? 'neutral'" />
      </button>
    </div>
  </div>
</template>
