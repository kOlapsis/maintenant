<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

withDefaults(
  defineProps<{
    ariaLabel: string
    align?: 'start' | 'end' | 'stretch'
    panelClass?: string
    panelRole?: string
  }>(),
  { align: 'end', panelClass: '', panelRole: 'menu' },
)

const open = defineModel<boolean>('open', { default: false })

function toggle() {
  open.value = !open.value
}
function close() {
  open.value = false
}

const rootRef = ref<HTMLElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)
let lastFocused: HTMLElement | null = null

function menuItems(): HTMLElement[] {
  if (!panelRef.value) return []
  return Array.from(
    panelRef.value.querySelectorAll<HTMLElement>(
      '[role="menuitem"]:not([aria-disabled="true"]), [role="menuitemcheckbox"]:not([aria-disabled="true"]), [role="menuitemradio"]:not([aria-disabled="true"])',
    ),
  )
}

function focusItem(index: number) {
  const items = menuItems()
  if (items.length === 0) return
  const wrapped = ((index % items.length) + items.length) % items.length
  items[wrapped]?.focus()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
    return
  }
  if (!open.value) return
  const items = menuItems()
  if (items.length === 0) return
  const current = items.indexOf(document.activeElement as HTMLElement)
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    focusItem(current + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    focusItem(current - 1)
  } else if (event.key === 'Home') {
    event.preventDefault()
    focusItem(0)
  } else if (event.key === 'End') {
    event.preventDefault()
    focusItem(items.length - 1)
  }
}

function onDocPointerDown(event: PointerEvent) {
  if (!open.value) return
  if (rootRef.value && !rootRef.value.contains(event.target as Node)) close()
}

onMounted(() => document.addEventListener('pointerdown', onDocPointerDown))
onUnmounted(() => document.removeEventListener('pointerdown', onDocPointerDown))

// Focus only follows into the panel when the trigger itself already had focus
// (a keyboard activation) — a hover-opened popover must not steal focus.
watch(open, async (isOpen) => {
  if (isOpen) {
    const activeInRoot = !!(rootRef.value && document.activeElement && rootRef.value.contains(document.activeElement))
    if (activeInRoot) {
      lastFocused = document.activeElement as HTMLElement
      await nextTick()
      focusItem(0)
    }
  } else if (lastFocused) {
    lastFocused.focus()
    lastFocused = null
  }
})

const alignClass: Record<'start' | 'end' | 'stretch', string> = {
  start: 'left-0',
  end: 'right-0',
  stretch: 'left-0 right-0',
}
</script>

<template>
  <div ref="rootRef" class="relative" @keydown="onKeydown">
    <slot name="trigger" :open="open" :toggle="toggle" />
    <Transition
      enter-active-class="transition duration-100 ease-out"
      enter-from-class="opacity-0 scale-95 -translate-y-1"
      enter-to-class="opacity-100 scale-100 translate-y-0"
      leave-active-class="transition duration-75 ease-in"
      leave-from-class="opacity-100 scale-100 translate-y-0"
      leave-to-class="opacity-0 scale-95 -translate-y-1"
    >
      <div
        v-if="open"
        ref="panelRef"
        :role="panelRole"
        :aria-label="ariaLabel"
        class="absolute top-full z-50 mt-1 overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface shadow-mnt-elevated"
        :class="[alignClass[align], panelClass]"
      >
        <slot />
      </div>
    </Transition>
  </div>
</template>
