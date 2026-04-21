<script setup lang="ts">
import InlineAlert from '@/components/ui/InlineAlert.vue'

defineProps<{
  title?: string
}>()
</script>

<template>
  <InlineAlert severity="warning" tag="SMTP">
    <template #title>{{ title ?? 'SMTP not configured' }}</template>
    Set the
    <code class="smtp-code mx-0.5 inline-flex items-center rounded border px-1.5 py-0.5 text-[11px] font-mono">MAINTENANT_SMTP_HOST</code>
    environment variable to enable email features.
  </InlineAlert>
</template>

<style scoped>
.smtp-code {
  background: var(--pb-alert-warn-tag-bg);
  border-color: var(--pb-alert-warn-tag-border);
  color: var(--pb-alert-warn-tag-text);
}
</style>
