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
import { computed } from 'vue'
import { AlertTriangle, ShieldAlert, Info, X } from 'lucide-vue-next'

type Severity = 'info' | 'warning' | 'critical'

const props = withDefaults(
  defineProps<{
    severity?: Severity
    label?: string
    compact?: boolean
    dismissible?: boolean
  }>(),
  {
    severity: 'warning',
    compact: false,
    dismissible: false,
  },
)

const emit = defineEmits<{ dismiss: [] }>()

const severityMeta = {
  info: { icon: Info, defaultLabel: 'INFO', key: 'info' },
  warning: { icon: AlertTriangle, defaultLabel: 'WARNING', key: 'warn' },
  critical: { icon: ShieldAlert, defaultLabel: 'CRITICAL', key: 'critical' },
} as const

const meta = computed(() => severityMeta[props.severity])
const labelText = computed(() => props.label ?? meta.value.defaultLabel)
</script>

<template>
  <div
    role="alert"
    class="alert-banner relative flex items-stretch overflow-hidden border-b"
    :class="`alert-banner--${meta.key}`"
  >
    <!-- Left accent rail -->
    <span class="alert-banner__rail w-[3px] shrink-0" />

    <!-- Gradient backdrop -->
    <span class="alert-banner__gradient absolute inset-0 pointer-events-none" />

    <!-- Diagonal scanline texture -->
    <span class="alert-banner__scan absolute inset-0 pointer-events-none" />

    <!-- Content -->
    <div
      class="relative flex-1 flex items-center gap-3 px-4"
      :class="compact ? 'py-1.5' : 'py-2.5'"
    >
      <!-- Pulsing signal dot -->
      <span class="relative flex items-center justify-center w-3 h-3 shrink-0">
        <span class="alert-banner__dot-halo absolute inline-flex h-3 w-3 rounded-full" />
        <span class="alert-banner__dot relative inline-flex h-[7px] w-[7px] rounded-full" />
      </span>

      <!-- Severity tag -->
      <span
        class="alert-banner__tag inline-flex items-center gap-1 shrink-0 rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-[0.18em] font-mono border"
      >
        <component :is="meta.icon" :size="10" :stroke-width="2.5" />
        <span>{{ labelText }}</span>
      </span>

      <!-- Message -->
      <div class="alert-banner__message flex-1 min-w-0 text-xs font-medium leading-snug">
        <slot />
      </div>

      <!-- Action slot -->
      <div v-if="$slots.action" class="shrink-0 flex items-center">
        <slot name="action" />
      </div>

      <!-- Dismiss -->
      <button
        v-if="dismissible"
        class="alert-banner__dismiss shrink-0 p-1 rounded transition-colors"
        aria-label="Dismiss"
        @click="emit('dismiss')"
      >
        <X :size="14" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.alert-banner--warn {
  border-color: var(--pb-alert-warn-border);
}
.alert-banner--critical {
  border-color: var(--pb-alert-critical-border);
}
.alert-banner--info {
  border-color: var(--pb-alert-info-border);
}

.alert-banner--warn .alert-banner__rail {
  background: var(--pb-alert-warn-rail);
}
.alert-banner--critical .alert-banner__rail {
  background: var(--pb-alert-critical-rail);
}
.alert-banner--info .alert-banner__rail {
  background: var(--pb-alert-info-rail);
}

.alert-banner--warn .alert-banner__gradient {
  background: var(--pb-alert-warn-gradient);
}
.alert-banner--critical .alert-banner__gradient {
  background: var(--pb-alert-critical-gradient);
}
.alert-banner--info .alert-banner__gradient {
  background: var(--pb-alert-info-gradient);
}

.alert-banner__scan {
  background-image: repeating-linear-gradient(
    135deg,
    var(--pb-alert-scan-color) 0px,
    var(--pb-alert-scan-color) 1px,
    transparent 1px,
    transparent 5px
  );
}

.alert-banner--warn .alert-banner__dot {
  background: var(--pb-alert-warn-dot);
  box-shadow: 0 0 6px var(--pb-alert-warn-dot-halo);
}
.alert-banner--critical .alert-banner__dot {
  background: var(--pb-alert-critical-dot);
  box-shadow: 0 0 6px var(--pb-alert-critical-dot-halo);
}
.alert-banner--info .alert-banner__dot {
  background: var(--pb-alert-info-dot);
  box-shadow: 0 0 6px var(--pb-alert-info-dot-halo);
}

.alert-banner--warn .alert-banner__dot-halo {
  background: var(--pb-alert-warn-dot-halo);
}
.alert-banner--critical .alert-banner__dot-halo {
  background: var(--pb-alert-critical-dot-halo);
}
.alert-banner--info .alert-banner__dot-halo {
  background: var(--pb-alert-info-dot-halo);
}

.alert-banner__dot-halo {
  animation: alert-ping 1.8s cubic-bezier(0, 0, 0.2, 1) infinite;
}

.alert-banner--warn .alert-banner__tag {
  background: var(--pb-alert-warn-tag-bg);
  border-color: var(--pb-alert-warn-tag-border);
  color: var(--pb-alert-warn-tag-text);
}
.alert-banner--critical .alert-banner__tag {
  background: var(--pb-alert-critical-tag-bg);
  border-color: var(--pb-alert-critical-tag-border);
  color: var(--pb-alert-critical-tag-text);
}
.alert-banner--info .alert-banner__tag {
  background: var(--pb-alert-info-tag-bg);
  border-color: var(--pb-alert-info-tag-border);
  color: var(--pb-alert-info-tag-text);
}

.alert-banner--warn .alert-banner__message {
  color: var(--pb-alert-warn-text);
}
.alert-banner--critical .alert-banner__message {
  color: var(--pb-alert-critical-text);
}
.alert-banner--info .alert-banner__message {
  color: var(--pb-alert-info-text);
}

.alert-banner__dismiss {
  color: var(--pb-text-muted);
}
.alert-banner__dismiss:hover {
  color: var(--pb-text-primary);
  background: var(--pb-bg-hover);
}

@keyframes alert-ping {
  0% {
    transform: scale(1);
    opacity: 0.75;
  }
  75%,
  100% {
    transform: scale(2.2);
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .alert-banner__dot-halo {
    animation: none;
    opacity: 0.4;
  }
}
</style>
