<script setup lang="ts">
import type { ContrastWarning, PalettePayload } from '@/services/personalizationApi'
import { usePersonalizationStore } from '@/stores/personalization'

const store = usePersonalizationStore()
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
    <h3 class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Color Palette</h3>

    <!-- Chrome colors -->
    <div class="space-y-3">
      <p class="text-xs text-slate-500">Chrome</p>
      <div v-for="field in chromeFields" :key="field.key" class="flex items-center gap-3">
        <input
          type="color"
          :value="palette?.[field.key]"
          class="w-8 h-8 rounded cursor-pointer border border-slate-700 bg-transparent"
          @input="(e: Event) => palette && (palette = { ...palette, [field.key]: (e.target as HTMLInputElement).value })"
        />
        <input
          type="text"
          :value="palette?.[field.key]"
          maxlength="9"
          class="w-28 bg-[#0B0E13] border border-slate-800 rounded px-2 py-1 text-white text-xs font-mono focus:outline-none focus:border-slate-600"
          @change="(e: Event) => palette && (palette = { ...palette, [field.key]: (e.target as HTMLInputElement).value })"
        />
        <span class="flex-1 text-xs text-slate-400">{{ field.label }}</span>
        <button
          class="text-[11px] text-slate-600 hover:text-slate-300"
          @click="resetField(field.key)"
        >
          Reset
        </button>
      </div>
    </div>

    <!-- Status colors -->
    <div class="space-y-3">
      <p class="text-xs text-slate-500">Status Indicators</p>
      <div v-for="field in statusFields" :key="field.key" class="flex items-center gap-3">
        <input
          type="color"
          :value="palette?.[field.key]"
          class="w-8 h-8 rounded cursor-pointer border border-slate-700 bg-transparent"
          @input="(e: Event) => palette && (palette = { ...palette, [field.key]: (e.target as HTMLInputElement).value })"
        />
        <input
          type="text"
          :value="palette?.[field.key]"
          maxlength="9"
          class="w-28 bg-[#0B0E13] border border-slate-800 rounded px-2 py-1 text-white text-xs font-mono focus:outline-none focus:border-slate-600"
          @change="(e: Event) => palette && (palette = { ...palette, [field.key]: (e.target as HTMLInputElement).value })"
        />
        <span class="flex-1 text-xs text-slate-400">{{ field.label }}</span>
        <button
          class="text-[11px] text-slate-600 hover:text-slate-300"
          @click="resetField(field.key)"
        >
          Reset
        </button>
      </div>
    </div>

    <!-- Contrast warnings -->
    <div v-if="warnings && warnings.length > 0" class="bg-yellow-950/30 border border-yellow-800/40 rounded-xl p-4 space-y-2">
      <p class="text-[10px] text-yellow-500 font-bold uppercase tracking-widest">WCAG AA Contrast Warnings</p>
      <div v-for="w in warnings" :key="w.pair" class="text-xs text-yellow-300">
        {{ w.pair.replace(/_/g, ' ') }}: {{ w.ratio }} (need ≥ {{ w.wcag_aa_threshold }})
      </div>
    </div>
  </div>
</template>
