<script setup lang="ts">
defineProps<{
  title?: string
}>()
</script>

<template>
  <div class="relative w-full rounded-xl border border-amber-500/30 bg-pb-surface px-5 py-5">
    <div class="min-w-0">
      <div class="flex items-center gap-2 mb-1">
        <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" class="text-amber-400 shrink-0">
          <rect x="2" y="4" width="16" height="12" rx="2" />
          <path d="M2 6l8 5 8-5" />
        </svg>
        <span class="text-sm font-semibold text-pb-secondary">{{ title ?? 'SMTP Configuration' }}</span>
      </div>
      <p class="text-xs leading-relaxed text-slate-500 pl-5.5">
        SMTP is not configured. Set the <code class="text-amber-400/80 bg-amber-500/10 px-1 py-0.5 rounded text-[10px]">MAINTENANT_SMTP_HOST</code> environment variable to enable email features.
      </p>
    </div>
  </div>
</template>
