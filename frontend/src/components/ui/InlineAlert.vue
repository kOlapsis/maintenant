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
import { AlertTriangle, ShieldAlert, Info, CheckCircle2 } from 'lucide-vue-next'

type Severity = 'info' | 'warning' | 'critical' | 'success'

const props = withDefaults(
  defineProps<{
    severity?: Severity
    title?: string
    tag?: string
  }>(),
  { severity: 'warning' },
)

const severityMeta = {
  info: { icon: Info, defaultTag: 'INFO', key: 'info' },
  success: { icon: CheckCircle2, defaultTag: 'OK', key: 'info' },
  warning: { icon: AlertTriangle, defaultTag: 'WARNING', key: 'warn' },
  critical: { icon: ShieldAlert, defaultTag: 'CRITICAL', key: 'critical' },
} as const

const meta = computed(() => severityMeta[props.severity])
const tagLabel = computed(() => props.tag ?? meta.value.defaultTag)
</script>

<template>
  <div
    role="alert"
    class="inline-alert relative overflow-hidden rounded-xl border bg-pb-surface"
    :class="`inline-alert--${meta.key}`"
  >
    <!-- Colored wash overlay -->
    <span class="inline-alert__wash absolute inset-0 pointer-events-none" />

    <!-- Left rail -->
    <span class="inline-alert__rail absolute top-0 bottom-0 left-0 w-[3px]" />

    <div class="relative flex items-start gap-4 p-4 pl-5">
      <!-- Icon medallion -->
      <div class="inline-alert__icon flex items-center justify-center w-10 h-10 rounded-xl border shrink-0">
        <component :is="meta.icon" :size="18" :stroke-width="2" />
      </div>

      <div class="flex-1 min-w-0">
        <!-- Title row -->
        <div class="flex items-center gap-2 flex-wrap mb-1">
          <h3 class="inline-alert__title text-sm font-semibold leading-tight">
            <slot name="title">{{ title }}</slot>
          </h3>
          <span v-if="tagLabel" class="inline-alert__tag inline-flex items-center rounded px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-[0.18em] font-mono border">
            {{ tagLabel }}
          </span>
        </div>

        <div class="inline-alert__body text-xs leading-relaxed">
          <slot />
        </div>

        <div v-if="$slots.action" class="mt-3 flex items-center gap-2">
          <slot name="action" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.inline-alert--warn {
  border-color: var(--pb-alert-warn-border);
}
.inline-alert--critical {
  border-color: var(--pb-alert-critical-border);
}
.inline-alert--info {
  border-color: var(--pb-alert-info-border);
}

.inline-alert--warn .inline-alert__wash {
  background: var(--pb-alert-warn-gradient-card);
}
.inline-alert--critical .inline-alert__wash {
  background: var(--pb-alert-critical-gradient-card);
}
.inline-alert--info .inline-alert__wash {
  background: var(--pb-alert-info-gradient-card);
}

.inline-alert--warn .inline-alert__rail {
  background: var(--pb-alert-warn-rail);
}
.inline-alert--critical .inline-alert__rail {
  background: var(--pb-alert-critical-rail);
}
.inline-alert--info .inline-alert__rail {
  background: var(--pb-alert-info-rail);
}

.inline-alert--warn .inline-alert__icon {
  background: var(--pb-alert-warn-icon-bg);
  border-color: var(--pb-alert-warn-icon-border);
  color: var(--pb-alert-warn-icon-color);
}
.inline-alert--critical .inline-alert__icon {
  background: var(--pb-alert-critical-icon-bg);
  border-color: var(--pb-alert-critical-icon-border);
  color: var(--pb-alert-critical-icon-color);
}
.inline-alert--info .inline-alert__icon {
  background: var(--pb-alert-info-icon-bg);
  border-color: var(--pb-alert-info-icon-border);
  color: var(--pb-alert-info-icon-color);
}

.inline-alert--warn .inline-alert__title {
  color: var(--pb-alert-warn-title);
}
.inline-alert--critical .inline-alert__title {
  color: var(--pb-alert-critical-title);
}
.inline-alert--info .inline-alert__title {
  color: var(--pb-alert-info-title);
}

.inline-alert--warn .inline-alert__tag {
  background: var(--pb-alert-warn-tag-bg);
  border-color: var(--pb-alert-warn-tag-border);
  color: var(--pb-alert-warn-tag-text);
}
.inline-alert--critical .inline-alert__tag {
  background: var(--pb-alert-critical-tag-bg);
  border-color: var(--pb-alert-critical-tag-border);
  color: var(--pb-alert-critical-tag-text);
}
.inline-alert--info .inline-alert__tag {
  background: var(--pb-alert-info-tag-bg);
  border-color: var(--pb-alert-info-tag-border);
  color: var(--pb-alert-info-tag-text);
}

.inline-alert__body {
  color: var(--pb-text-secondary);
}

.inline-alert--warn .inline-alert__body {
  color: var(--pb-alert-warn-text);
}
.inline-alert--critical .inline-alert__body {
  color: var(--pb-alert-critical-text);
}
.inline-alert--info .inline-alert__body {
  color: var(--pb-alert-info-text);
}
</style>
