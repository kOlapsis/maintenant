<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { SlidersHorizontal, Loader2 } from 'lucide-vue-next'
import RangeInput from '@/components/ui/RangeInput.vue'
import type { AnomalySettings } from '@/commercial/services/anomalyApi'

const props = defineProps<{
  settings: AnomalySettings
  saving?: boolean
}>()

const emit = defineEmits<{ (e: 'change', value: number): void }>()

// The slider reads left to right as "trust the hour more", so it runs backwards against the stored pull.
const local = ref(props.settings.max_bucket_pull - props.settings.bucket_pull)
watch(
  () => props.settings.bucket_pull,
  (v) => {
    local.value = props.settings.max_bucket_pull - v
  },
)

const pull = computed(() => props.settings.max_bucket_pull - local.value)

const retained = computed(() => {
  const n = props.settings.min_samples
  return Math.round((n / (n + pull.value)) * 100)
})

const summary = computed(() => {
  if (pull.value === 0) return 'Each hour of the week decides entirely on its own measurements.'
  if (retained.value >= 80) return 'Hours keep most of their own level; weekly rhythms stay sharp.'
  if (retained.value >= 40) return 'Hours are balanced against the container’s usual level.'
  return 'Hours barely differ; only the container’s usual level counts.'
})
</script>

<template>
  <div class="rounded-xl border border-mnt-default bg-mnt-surface p-4">
    <div class="flex items-center gap-2">
      <SlidersHorizontal :size="16" class="text-mnt-muted" />
      <h3 class="text-sm font-semibold text-mnt-primary">Seasonality trust</h3>
      <Loader2 v-if="saving" :size="14" class="ml-auto animate-spin text-mnt-muted" />
    </div>

    <p class="mt-1.5 text-xs leading-relaxed text-mnt-muted">
      A given hour of the week is only seen {{ settings.min_samples }} times in a full baseline window.
      On a volatile metric that is too few to call it a rhythm, so its level can be balanced against
      what the container usually does.
    </p>

    <RangeInput
      v-model="local"
      label="Seasonality trust"
      class="mt-3"
      :min="settings.min_bucket_pull"
      :max="settings.max_bucket_pull"
      :disabled="saving"
      @change="emit('change', pull)"
    />

    <div class="flex justify-between text-[11px] text-mnt-muted">
      <span>Container level</span>
      <span>Hour of the week</span>
    </div>

    <p class="mt-2 text-xs text-mnt-secondary">
      {{ summary }}
      <span v-if="pull > 0" class="text-mnt-muted">
        A fully observed hour keeps {{ retained }}% of its own level.
      </span>
    </p>
  </div>
</template>
