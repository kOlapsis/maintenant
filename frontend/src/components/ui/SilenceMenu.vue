<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { createSilenceRule, type CreateSilenceRuleInput } from '@/services/alertApi'

const props = defineProps<{
  entityType?: string
  entityId?: number
  source?: string
}>()

const emit = defineEmits<{
  silenced: []
}>()

const open = ref(false)
const submitting = ref(false)
const customHours = ref(1)
const showCustom = ref(false)
const reason = ref('')
const menuRef = ref<HTMLElement | null>(null)

const presets = [
  { label: '1 hour', seconds: 3600 },
  { label: '4 hours', seconds: 14400 },
  { label: '8 hours', seconds: 28800 },
  { label: '24 hours', seconds: 86400 },
]

async function silence(durationSeconds: number) {
  submitting.value = true
  try {
    const data: CreateSilenceRuleInput = { duration_seconds: durationSeconds }
    if (props.entityType) data.entity_type = props.entityType
    if (props.entityId) data.entity_id = props.entityId
    if (props.source) data.source = props.source
    if (reason.value) data.reason = reason.value
    await createSilenceRule(data)
    open.value = false
    showCustom.value = false
    reason.value = ''
    emit('silenced')
  } catch (e) {
    console.error('Failed to create silence rule:', e)
  } finally {
    submitting.value = false
  }
}

function silenceCustom() {
  const seconds = customHours.value * 3600
  silence(seconds)
}

function handleClickOutside(e: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    open.value = false
    showCustom.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside, true)
})
</script>

<template>
  <div ref="menuRef" class="relative inline-block">
    <!-- Trigger button -->
    <button
      @click.stop="open = !open"
      class="flex items-center gap-1 rounded px-2 py-1 text-xs transition-colors"
      :style="{
        background: open ? 'var(--pb-bg-hover)' : 'transparent',
        color: 'var(--pb-text-muted)',
      }"
      @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--pb-bg-hover)'"
      @mouseleave="($event.currentTarget as HTMLElement).style.background = open ? 'var(--pb-bg-hover)' : 'transparent'"
      title="Silence alerts"
    >
      <!-- Bell-off icon -->
      <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
        <path d="M5.5 12.5c.5.5 1 .5 1.5.5s1 0 1.5-.5" />
        <path d="M3.5 5.5a3.5 3.5 0 0 1 7 0c0 3.5 1.5 4.5 1.5 4.5H2S3.5 9 3.5 5.5" />
        <line x1="1" y1="1" x2="13" y2="13" />
      </svg>
      Silence
    </button>

    <!-- Dropdown menu -->
    <div
      v-if="open"
      class="absolute right-0 z-50 mt-1 w-56 rounded-lg border shadow-lg"
      style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); box-shadow: var(--pb-shadow-elevated)"
    >
      <div class="p-2">
        <p class="mb-2 px-2 text-xs font-medium" style="color: var(--pb-text-muted)">Silence for</p>

        <!-- Preset durations -->
        <button
          v-for="preset in presets"
          :key="preset.seconds"
          @click="silence(preset.seconds)"
          :disabled="submitting"
          class="flex w-full items-center rounded px-2 py-1.5 text-sm transition-colors disabled:opacity-50"
          style="color: var(--pb-text-secondary)"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--pb-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
        >
          {{ preset.label }}
        </button>

        <!-- Divider -->
        <div class="my-1 h-px" style="background: var(--pb-border-subtle)"></div>

        <!-- Custom toggle -->
        <button
          v-if="!showCustom"
          @click.stop="showCustom = true"
          class="flex w-full items-center rounded px-2 py-1.5 text-sm transition-colors"
          style="color: var(--pb-text-secondary)"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--pb-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
        >
          Custom duration...
        </button>

        <!-- Custom input -->
        <div v-if="showCustom" class="space-y-2 px-2 py-1.5">
          <div class="flex items-center gap-2">
            <input
              v-model.number="customHours"
              type="number"
              min="1"
              max="720"
              class="w-16 rounded border px-2 py-1 text-sm outline-none"
              style="background: var(--pb-bg-surface); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            />
            <span class="text-xs" style="color: var(--pb-text-muted)">hours</span>
          </div>
          <input
            v-model="reason"
            placeholder="Reason (optional)"
            class="w-full rounded border px-2 py-1 text-xs outline-none"
            style="background: var(--pb-bg-surface); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          />
          <button
            @click="silenceCustom"
            :disabled="submitting"
            class="w-full rounded px-2 py-1 text-xs font-medium text-white disabled:opacity-50"
            style="background: var(--pb-accent)"
          >
            {{ submitting ? 'Creating...' : 'Apply Silence' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
