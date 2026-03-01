<script setup lang="ts">
import { ExternalLink, AlertTriangle } from 'lucide-vue-next'

const props = defineProps<{
  changelogUrl: string
  changelogSummary: string
  hasBreakingChanges: boolean
  sourceUrl?: string
}>()
</script>

<template>
  <div class="bg-[#0f1115] rounded-xl p-4 border border-slate-800">
    <div class="flex items-center justify-between mb-2">
      <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Changelog</h4>
      <a
        v-if="changelogUrl"
        :href="changelogUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="text-[10px] text-blue-500 hover:text-blue-400 flex items-center gap-1 transition-colors"
      >
        View release <ExternalLink :size="9" />
      </a>
    </div>

    <div v-if="hasBreakingChanges" class="flex items-center gap-1.5 mb-2 px-2 py-1 bg-rose-500/10 rounded-lg border border-rose-500/20">
      <AlertTriangle :size="11" class="text-rose-400" />
      <span class="text-[10px] font-bold text-rose-400 uppercase">Breaking Changes</span>
    </div>

    <p v-if="changelogSummary" class="text-xs text-slate-400 leading-relaxed">
      {{ changelogSummary }}
    </p>
    <p v-else class="text-xs text-slate-600 italic">
      No summary available
    </p>

    <a
      v-if="sourceUrl"
      :href="sourceUrl"
      target="_blank"
      rel="noopener noreferrer"
      class="mt-2 text-[10px] text-slate-500 hover:text-slate-400 flex items-center gap-1 transition-colors"
    >
      Source code <ExternalLink :size="9" />
    </a>
  </div>
</template>
