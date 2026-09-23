<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from 'lucide-vue-next'
import AlertBanner from '@/components/ui/AlertBanner.vue'
import { useEdition } from '@/composables/useEdition'

const { suspendedChannels } = useEdition()

const title = computed(() => {
  const n = suspendedChannels.value.length
  return `${n} notification channel${n === 1 ? '' : 's'} suspended`
})

function editionLabel(edition: string): string {
  return edition ? edition.charAt(0).toUpperCase() + edition.slice(1) : ''
}
</script>

<template>
  <AlertBanner
    v-if="suspendedChannels.length > 0"
    severity="critical"
    label="CHANNELS SUSPENDED"
    data-test="suspended-channels-banner"
  >
    <strong class="font-semibold">{{ title }}.</strong>
    Alerts are no longer delivered through
    <template v-for="(ch, i) in suspendedChannels" :key="ch.id">
      <span data-test="suspended-channel">{{ ch.name }} ({{ ch.type }}, requires {{ editionLabel(ch.required_edition) }})</span><template v-if="i < suspendedChannels.length - 1">, </template>
    </template>.
    Upgrade your edition, or move these alerts to a channel your edition includes.
    <template #action>
      <div class="flex items-center gap-2">
        <RouterLink
          to="/channels"
          class="suspended-action inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
        >
          Channels
          <ArrowRight :size="12" />
        </RouterLink>
        <RouterLink
          to="/editions"
          class="suspended-action inline-flex items-center gap-1 rounded border px-2 py-0.5 text-[11px] font-semibold transition-colors"
        >
          Editions
          <ArrowRight :size="12" />
        </RouterLink>
      </div>
    </template>
  </AlertBanner>
</template>

<style scoped>
.suspended-action {
  background: var(--mnt-alert-critical-action-bg);
  border-color: var(--mnt-alert-critical-action-border);
  color: var(--mnt-alert-critical-action-text);
}
.suspended-action:hover {
  background: var(--mnt-alert-critical-action-hover);
}
</style>
