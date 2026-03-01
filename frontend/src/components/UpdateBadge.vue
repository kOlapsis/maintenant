<script setup lang="ts">
import type { ImageUpdate } from '@/services/updateApi'
import { computed } from 'vue'
import { ArrowRight, Check, Pin } from 'lucide-vue-next'

const props = defineProps<{
  update: ImageUpdate | null
}>()

const badgeConfig = computed(() => {
  if (!props.update) return null

  if (props.update.status === 'pinned') {
    return { color: 'var(--pb-text-muted)', bg: 'var(--pb-bg-elevated)', icon: Pin, label: 'Pinned' }
  }

  switch (props.update.update_type) {
    case 'major':
      return { color: 'var(--pb-status-down)', bg: 'var(--pb-status-down-bg)', icon: ArrowRight, label: `${props.update.current_tag} → ${props.update.latest_tag}` }
    case 'minor':
      return { color: 'var(--pb-status-warn)', bg: 'var(--pb-status-warn-bg)', icon: ArrowRight, label: `${props.update.current_tag} → ${props.update.latest_tag}` }
    case 'patch':
      return { color: '#3b82f6', bg: 'rgba(59,130,246,0.1)', icon: ArrowRight, label: `${props.update.current_tag} → ${props.update.latest_tag}` }
    case 'digest_only':
      return { color: 'var(--pb-text-muted)', bg: 'var(--pb-bg-elevated)', icon: ArrowRight, label: 'New digest' }
    default:
      return { color: 'var(--pb-text-muted)', bg: 'var(--pb-bg-elevated)', icon: ArrowRight, label: 'Update' }
  }
})
</script>

<template>
  <div v-if="!update" class="inline-flex items-center gap-1 text-[10px]" :style="{ color: 'var(--pb-status-ok)' }">
    <Check :size="10" />
    <span class="font-medium">Up to date</span>
  </div>
  <div
    v-else-if="badgeConfig"
    class="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium"
    :style="{ color: badgeConfig.color, backgroundColor: badgeConfig.bg }"
  >
    <component :is="badgeConfig.icon" :size="9" />
    <span class="truncate max-w-[120px]">{{ badgeConfig.label }}</span>
  </div>
</template>
