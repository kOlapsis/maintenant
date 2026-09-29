<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import type { ContrastWarning, PalettePayload } from '@/commercial/services/personalizationApi'
import UiButton from '@/components/ui/UiButton.vue'
import ColorInput from '@/components/ui/ColorInput.vue'

const palette = defineModel<PalettePayload>('palette', { required: true })
const warnings = defineModel<ContrastWarning[]>('warnings', { default: () => [] })

const defaults: PalettePayload = {
  bg: '#0B0E13',
  surface: '#12151C',
  border: '#1F2937',
  text: '#FFFFFF',
  accent: '#22C55E',
  status_operational: '#22C55E',
  status_degraded: '#EAB308',
  status_partial: '#F97316',
  status_major: '#EF4444',
}

type PaletteKey = keyof PalettePayload

const chromeFields: { key: PaletteKey; label: string }[] = [
  { key: 'bg', label: 'Background' },
  { key: 'surface', label: 'Surface (cards)' },
  { key: 'border', label: 'Border' },
  { key: 'text', label: 'Text' },
  { key: 'accent', label: 'Accent' },
]

const statusFields: { key: PaletteKey; label: string }[] = [
  { key: 'status_operational', label: 'Operational' },
  { key: 'status_degraded', label: 'Degraded' },
  { key: 'status_partial', label: 'Partial Outage' },
  { key: 'status_major', label: 'Major Outage' },
]

function resetField(key: PaletteKey) {
  if (palette.value) {
    palette.value = { ...palette.value, [key]: defaults[key] }
  }
}
</script>

<template>
  <div class="space-y-6">
    <h3 class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Color Palette</h3>

    <!-- Chrome colors -->
    <div class="space-y-3">
      <p class="text-xs text-mnt-muted">Chrome</p>
      <div v-for="field in chromeFields" :key="field.key" class="flex items-center gap-3">
        <ColorInput
          :model-value="palette?.[field.key]"
          :label="field.label"
          @update:model-value="(v) => palette && (palette = { ...palette, [field.key]: v })"
        />
        <span class="flex-1 text-xs text-mnt-muted">{{ field.label }}</span>
        <UiButton variant="ghost" size="sm" @click="resetField(field.key)">Reset</UiButton>
      </div>
    </div>

    <!-- Status colors -->
    <div class="space-y-3">
      <p class="text-xs text-mnt-muted">Status Indicators</p>
      <div v-for="field in statusFields" :key="field.key" class="flex items-center gap-3">
        <ColorInput
          :model-value="palette?.[field.key]"
          :label="field.label"
          @update:model-value="(v) => palette && (palette = { ...palette, [field.key]: v })"
        />
        <span class="flex-1 text-xs text-mnt-muted">{{ field.label }}</span>
        <UiButton variant="ghost" size="sm" @click="resetField(field.key)">Reset</UiButton>
      </div>
    </div>

    <!-- Contrast warnings -->
    <div v-if="warnings && warnings.length > 0" class="rounded-xl border border-mnt-sev-warning bg-mnt-status-warn p-4 space-y-2">
      <p class="text-[10px] text-mnt-status-warn font-bold uppercase tracking-widest">WCAG AA Contrast Warnings</p>
      <div v-for="w in warnings" :key="w.pair" class="text-xs text-mnt-status-warn">
        {{ w.pair.replace(/_/g, ' ') }}: {{ w.ratio }} (need ≥ {{ w.wcag_aa_threshold }})
      </div>
    </div>
  </div>
</template>
