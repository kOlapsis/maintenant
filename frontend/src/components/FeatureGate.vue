<script setup lang="ts">
import { computed } from 'vue'
import { useEdition } from '@/composables/useEdition'
import { Lock, Sparkles } from 'lucide-vue-next'

const props = defineProps<{
  feature: string
  title?: string
  description?: string
}>()

const { edition } = useEdition()
const enabled = computed(() => edition.value?.features[props.feature] === true)
</script>

<template>
  <slot v-if="enabled" />
  <template v-else-if="enabled === false">
    <slot name="placeholder">
      <!-- Default placeholder when no custom one is provided -->
      <div v-if="title" class="relative w-full rounded-xl border border-zinc-800 bg-[#151923] px-5 py-5">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <Sparkles class="h-3.5 w-3.5 text-indigo-400 shrink-0" />
              <span class="text-sm font-semibold text-zinc-300">{{ title }}</span>
            </div>
            <p v-if="description" class="text-xs leading-relaxed text-zinc-500 pl-5.5">
              {{ description }}
            </p>
          </div>
          <div class="flex items-center gap-1.5 shrink-0 mt-0.5">
            <Lock class="h-3 w-3 text-indigo-400/60" />
            <span class="rounded-full bg-indigo-600/15 px-2.5 py-0.5 text-[10px] font-semibold text-indigo-400">
              Pro
            </span>
          </div>
        </div>
      </div>
    </slot>
  </template>
</template>
