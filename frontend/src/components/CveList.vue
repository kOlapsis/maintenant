<script setup lang="ts">
import type { CVEInfo } from '@/services/updateApi'
import { computed } from 'vue'
import { Shield, ExternalLink } from 'lucide-vue-next'

const props = defineProps<{
  cves: CVEInfo[]
}>()

const severityColors: Record<string, { bg: string; text: string }> = {
  critical: { bg: 'bg-rose-500/10', text: 'text-rose-400' },
  high: { bg: 'bg-orange-500/10', text: 'text-orange-400' },
  medium: { bg: 'bg-amber-500/10', text: 'text-amber-400' },
  low: { bg: 'bg-slate-500/10', text: 'text-slate-400' },
}

function getSeverityStyle(sev: string) {
  return severityColors[sev] || severityColors.low
}
</script>

<template>
  <div v-if="cves.length === 0" class="text-xs text-slate-600 py-4 text-center">
    Aucune CVE active
  </div>
  <div v-else class="space-y-2">
    <div
      v-for="cve in cves"
      :key="cve.cve_id"
      class="bg-[#0f1115] rounded-xl p-3 border border-slate-800"
    >
      <div class="flex items-center justify-between mb-1">
        <div class="flex items-center gap-2">
          <Shield :size="11" :class="getSeverityStyle(cve.severity).text" />
          <span class="text-xs font-bold text-slate-200">{{ cve.cve_id }}</span>
          <span
            :class="[
              'px-1.5 py-0.5 rounded text-[9px] font-bold uppercase',
              getSeverityStyle(cve.severity).bg,
              getSeverityStyle(cve.severity).text,
            ]"
          >{{ cve.severity }}</span>
        </div>
        <span class="text-[10px] font-mono text-slate-400">CVSS {{ cve.cvss_score?.toFixed(1) || 'N/A' }}</span>
      </div>
      <p v-if="cve.summary" class="text-[11px] text-slate-500 mt-1">{{ cve.summary }}</p>
      <p v-if="cve.fixed_in" class="text-[10px] text-emerald-500 mt-1 font-medium">
        Corrigé dans : {{ cve.fixed_in }}
      </p>
    </div>
  </div>
</template>
