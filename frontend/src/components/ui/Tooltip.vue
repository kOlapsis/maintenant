<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { computed, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    text?: string
    placement?: 'top' | 'bottom' | 'left' | 'right'
  }>(),
  { placement: 'top' },
)

const open = ref(false)
const coords = ref({ x: 0, y: 0 })
const triggerRef = ref<HTMLElement | null>(null)

function show() {
  const el = triggerRef.value
  if (!el) return
  const r = el.getBoundingClientRect()
  const cx = r.left + r.width / 2
  const cy = r.top + r.height / 2
  coords.value =
    props.placement === 'top'
      ? { x: cx, y: r.top }
      : props.placement === 'bottom'
        ? { x: cx, y: r.bottom }
        : props.placement === 'left'
          ? { x: r.left, y: cy }
          : { x: r.right, y: cy }
  open.value = true
}

function hide() {
  open.value = false
}

const tipStyle = computed(() => {
  const gap = 8
  const base: Record<string, string> = {
    left: `${coords.value.x}px`,
    top: `${coords.value.y}px`,
  }
  const transforms: Record<'top' | 'bottom' | 'left' | 'right', string> = {
    top: `translate(-50%, calc(-100% - ${gap}px))`,
    bottom: `translate(-50%, ${gap}px)`,
    left: `translate(calc(-100% - ${gap}px), -50%)`,
    right: `translate(${gap}px, -50%)`,
  }
  base.transform = transforms[props.placement]
  return base
})
</script>

<template>
  <span
    ref="triggerRef"
    class="inline-flex"
    @mouseenter="show"
    @mouseleave="hide"
    @focusin="show"
    @focusout="hide"
  >
    <slot name="trigger"><slot /></slot>
  </span>
  <Teleport to="body">
    <div v-if="open" class="mnt-tooltip" :style="tipStyle" role="tooltip">
      <slot name="content">{{ text }}</slot>
    </div>
  </Teleport>
</template>

<style scoped>
.mnt-tooltip {
  position: fixed;
  z-index: 60;
  max-width: 240px;
  padding: 6px 10px;
  font-size: 11.5px;
  line-height: 1.4;
  color: var(--mnt-text-primary);
  background: var(--mnt-bg-elevated);
  border: 1px solid var(--mnt-border-default);
  border-radius: var(--mnt-radius-md);
  box-shadow: var(--mnt-shadow-elevated);
  pointer-events: none;
  white-space: normal;
}
</style>
