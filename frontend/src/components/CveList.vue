<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See LICENSE-COMMERCIAL.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import type { CVEInfo } from '@/services/updateApi'
import { ref } from 'vue'
import { Shield, Copy, Check, CheckCircle } from 'lucide-vue-next'

const props = defineProps<{
  cves: CVEInfo[]
}>()

const severityColors: Record<string, { bg: string; text: string }> = {
  critical: { bg: 'bg-rose-500/10', text: 'text-rose-400' },
  high: { bg: 'bg-orange-500/10', text: 'text-orange-400' },
  medium: { bg: 'bg-amber-500/10', text: 'text-amber-400' },
  low: { bg: 'bg-slate-500/10', text: 'text-slate-400' },
}

function getSeverityStyle(sev: string): { bg: string; text: string } {
  return severityColors[sev] ?? severityColors['low']!
}

const copiedFixId = ref<string | null>(null)

async function copyFixCommand(cveId: string, command: string) {
  try {
    await navigator.clipboard.writeText(command)
    copiedFixId.value = cveId
    setTimeout(() => { copiedFixId.value = null }, 2000)
  } catch {
    // fallback
  }
}
</script>

<template>
  <div v-if="cves.length === 0" class="text-xs text-slate-600 py-4 text-center">
    No active CVEs
  </div>
  <div v-else class="space-y-2">
    <div
      v-for="cve in cves"
      :key="cve.cve_id"
      class="bg-[#0B0E13] rounded-xl p-3 border border-slate-800"
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
      <div v-if="cve.fixed_in" class="mt-1.5">
        <div class="flex items-center gap-2">
          <p class="text-[10px] text-emerald-500 font-medium">
            Fixed in: {{ cve.fixed_in }}
          </p>
          <span
            v-if="cve.is_fixed_by_update"
            class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 flex items-center gap-0.5"
          >
            <CheckCircle :size="8" />
            Covered by update
          </span>
        </div>
        <div v-if="cve.fix_command && !cve.is_fixed_by_update" class="mt-1.5">
          <div class="flex items-center justify-between mb-1">
            <span class="text-[9px] text-slate-600 uppercase tracking-wider">Fix command</span>
            <button
              @click="copyFixCommand(cve.cve_id, cve.fix_command)"
              class="text-[9px] text-emerald-500 hover:text-emerald-400 flex items-center gap-1 transition-colors"
            >
              <component :is="copiedFixId === cve.cve_id ? Check : Copy" :size="9" />
              {{ copiedFixId === cve.cve_id ? 'Copied!' : 'Copy' }}
            </button>
          </div>
          <pre class="text-[10px] text-slate-300 bg-[#0a0c10] rounded-lg p-2 overflow-x-auto font-mono">{{ cve.fix_command }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
