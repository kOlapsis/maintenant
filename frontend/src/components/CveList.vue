<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<script setup lang="ts">
import type { CVEInfo } from '@/services/updateApi'
import { ref } from 'vue'
import { Shield, Copy, Check, CheckCircle } from 'lucide-vue-next'

defineProps<{
  cves: CVEInfo[]
}>()

const severityColors: Record<string, { bg: string; text: string }> = {
  critical: { bg: 'bg-mnt-status-down', text: 'text-mnt-status-down' },
  high: { bg: 'bg-orange-500/10', text: 'text-mnt-status-warn' },
  medium: { bg: 'bg-mnt-status-warn', text: 'text-mnt-status-warn' },
  low: { bg: 'bg-mnt-sev-neutral-solid/10', text: 'text-mnt-muted' },
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
  <div v-if="cves.length === 0" class="text-xs text-mnt-muted py-4 text-center">
    No active CVEs
  </div>
  <div v-else class="space-y-2">
    <div
      v-for="cve in cves"
      :key="cve.cve_id"
      class="bg-mnt-primary rounded-xl p-3 border border-mnt-default"
    >
      <div class="flex items-center justify-between mb-1">
        <div class="flex items-center gap-2">
          <Shield :size="11" :class="getSeverityStyle(cve.severity).text" />
          <span class="text-xs font-bold text-mnt-primary">{{ cve.cve_id }}</span>
          <span
            :class="[
              'px-1.5 py-0.5 rounded text-[9px] font-bold uppercase',
              getSeverityStyle(cve.severity).bg,
              getSeverityStyle(cve.severity).text,
            ]"
          >{{ cve.severity }}</span>
        </div>
        <span class="text-[10px] font-mono text-mnt-muted">CVSS {{ cve.cvss_score?.toFixed(1) || 'N/A' }}</span>
      </div>
      <p v-if="cve.summary" class="text-[11px] text-mnt-muted mt-1">{{ cve.summary }}</p>
      <div v-if="cve.fixed_in" class="mt-1.5">
        <div class="flex items-center gap-2">
          <p class="text-[10px] text-mnt-status-ok font-medium">
            Fixed in: {{ cve.fixed_in }}
          </p>
          <span
            v-if="cve.is_fixed_by_update"
            class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded bg-mnt-status-ok text-mnt-status-ok flex items-center gap-0.5"
          >
            <CheckCircle :size="8" />
            Covered by update
          </span>
        </div>
        <div v-if="cve.fix_command && !cve.is_fixed_by_update" class="mt-1.5">
          <div class="flex items-center justify-between mb-1">
            <span class="text-[9px] text-mnt-muted uppercase tracking-wider">Fix command</span>
            <button
              @click="copyFixCommand(cve.cve_id, cve.fix_command)"
              class="text-[9px] text-mnt-status-ok hover:text-mnt-accent flex items-center gap-1 transition-colors"
              aria-label="Copy fix command"
            >
              <component :is="copiedFixId === cve.cve_id ? Check : Copy" :size="9" />
              {{ copiedFixId === cve.cve_id ? 'Copied!' : 'Copy' }}
            </button>
          </div>
          <pre class="text-[10px] text-mnt-secondary bg-mnt-primary rounded-lg p-2 overflow-x-auto font-mono">{{ cve.fix_command }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
