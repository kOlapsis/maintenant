<script setup lang="ts">
import { computed } from 'vue'
import { useEdition } from '@/composables/useEdition'
import { Lock } from 'lucide-vue-next'

const props = defineProps<{
  feature: string
  title?: string
}>()

const { edition } = useEdition()
const enabled = computed(() => edition.value?.features[props.feature] === true)
</script>

<template>
  <slot v-if="enabled" />
  <template v-else-if="enabled === false">
    <slot name="placeholder">
      <!-- Default placeholder when no custom one is provided -->
      <div v-if="title" class="relative w-full rounded-xl border border-zinc-800 bg-[#151923] px-5 py-4">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-zinc-500">{{ title }}</span>
          <div class="flex items-center gap-1.5">
            <Lock class="h-3.5 w-3.5 text-zinc-500" />
            <span class="rounded-full bg-indigo-600/20 px-2.5 py-0.5 text-[10px] font-semibold text-indigo-400">
              Pro coming soon
            </span>
          </div>
        </div>
      </div>
    </slot>
  </template>
</template>
