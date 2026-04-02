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
import { computed, onMounted, ref } from 'vue'
import { useUpdatesStore } from '@/stores/updates'
import { type ContainerUpdateDetail, fetchContainerUpdate } from '@/services/updateApi'
import { useEdition } from '@/composables/useEdition'
import RiskScoreGauge from '@/components/RiskScoreGauge.vue'
import CveList from '@/components/CveList.vue'
import ChangelogViewer from '@/components/ChangelogViewer.vue'
import FeatureGate from '@/components/FeatureGate.vue'
import { AlertTriangle, ArrowRight, Check, Copy, ExternalLink, Pin, PinOff } from 'lucide-vue-next'

const { hasFeature } = useEdition()

const props = defineProps<{
  containerId: string
}>()

const updates = useUpdatesStore()
const detail = ref<ContainerUpdateDetail | null>(null)
const loading = ref(true)
const copied = ref(false)
const copiedRollback = ref(false)
const pinReason = ref('')
const showPinInput = ref(false)

const riskLevel = computed(() => {
  if (!detail.value) return 'low'
  const s = detail.value.risk_score
  if (s >= 81) return 'critical'
  if (s >= 61) return 'high'
  if (s >= 31) return 'moderate'
  return 'low'
})

async function loadDetail() {
  loading.value = true
  try {
    detail.value = await fetchContainerUpdate(props.containerId)
    if (hasFeature('cve_enrichment')) {
      await updates.fetchContainerCves(props.containerId)
    }
  } catch {
    // ignore
  } finally {
    loading.value = false
  }
}

async function copyCommand() {
  if (!detail.value?.update_command) return
  try {
    await navigator.clipboard.writeText(detail.value.update_command)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // fallback
  }
}

async function copyRollbackCommand() {
  if (!detail.value?.rollback_command) return
  try {
    await navigator.clipboard.writeText(detail.value.rollback_command)
    copiedRollback.value = true
    setTimeout(() => {
      copiedRollback.value = false
    }, 2000)
  } catch {
    // fallback
  }
}

async function handlePin() {
  if (detail.value?.pinned) {
    await updates.unpinVersion(props.containerId)
  } else {
    if (!showPinInput.value) {
      showPinInput.value = true
      return
    }
    await updates.pinVersion(props.containerId, pinReason.value)
    showPinInput.value = false
    pinReason.value = ''
  }
  await loadDetail()
}

onMounted(loadDetail)
</script>

<template>
  <div v-if="loading" class="flex items-center justify-center py-12">
    <div
      class="w-6 h-6 border-2 border-pb-green-500 border-t-transparent rounded-full animate-spin"
    />
  </div>

  <div v-else-if="detail" class="space-y-5">
    <!-- 1. Version info -->
    <div class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">Version</h4>
      <div class="flex items-center gap-3">
        <div class="text-center">
          <p class="text-xs text-slate-500 mb-0.5">Current</p>
          <p class="text-sm font-bold text-pb-primary font-mono">
            {{ detail.current_tag || 'latest' }}
          </p>
        </div>
        <ArrowRight :size="16" class="text-pb-green-500 shrink-0" />
        <div class="text-center">
          <p class="text-xs text-slate-500 mb-0.5">Available</p>
          <p class="text-sm font-bold text-pb-green-400 font-mono">{{ detail.latest_tag }}</p>
        </div>
      </div>
      <div class="mt-2 flex items-center gap-2">
        <span
          class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded"
          :class="{
            'bg-pb-status-down text-pb-status-down': detail.update_type === 'major',
            'bg-amber-500/10 text-amber-400': detail.update_type === 'minor',
            'bg-pb-green-500/10 text-pb-green-400': detail.update_type === 'patch',
            'bg-slate-500/10 text-slate-400': detail.update_type === 'digest_only',
          }"
          >{{ detail.update_type }}</span
        >
        <span
          v-if="detail.pinned"
          class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded bg-slate-500/10 text-slate-400"
        >
          <Pin :size="8" class="inline mr-0.5" /> Pinned
        </span>
      </div>
      <p v-if="detail.pinned && detail.pin_reason" class="mt-2 text-xs text-slate-400 italic">
        {{ detail.pin_reason }}
      </p>
    </div>

    <!-- 2. Tag Filter (when configured) -->
    <div
      v-if="detail.tag_include || detail.tag_exclude"
      class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
    >
      <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">
        Tag Filter
      </h4>
      <div class="space-y-2">
        <div v-if="detail.tag_include" class="flex items-start gap-2">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest shrink-0 pt-0.5 w-14">Include</span>
          <code class="text-[11px] text-pb-green-400 font-mono bg-pb-green-500/5 px-2 py-0.5 rounded break-all">{{ detail.tag_include }}</code>
        </div>
        <div v-if="detail.tag_exclude" class="flex items-start gap-2">
          <span class="text-[10px] font-bold text-slate-500 uppercase tracking-widest shrink-0 pt-0.5 w-14">Exclude</span>
          <code class="text-[11px] text-rose-400 font-mono bg-rose-500/5 px-2 py-0.5 rounded break-all">{{ detail.tag_exclude }}</code>
        </div>
      </div>
    </div>

    <!-- 3. Update command -->
    <div v-if="detail.update_command" class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <div class="flex items-center justify-between mb-2">
        <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest">
          Update Command
        </h4>
        <button
          @click="copyCommand"
          class="text-[10px] text-pb-green-500 hover:text-pb-green-400 flex items-center gap-1 transition-colors"
          aria-label="Copy update command"
        >
          <component :is="copied ? Check : Copy" :size="10" />
          {{ copied ? 'Copied!' : 'Copy' }}
        </button>
      </div>
      <pre
        class="text-[11px] rounded-lg p-3 overflow-x-auto font-mono whitespace-pre" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
        >{{ detail.update_command }}</pre
      >
      <p
        v-if="detail.update_command.includes('<compose-project-dir>')"
        class="text-[9px] text-slate-600 mt-2"
      >
        Replace &lt;compose-project-dir&gt; with the actual path to your docker-compose.yml
        directory.
      </p>
    </div>

    <!-- 4. Breaking changes warning (Pro) -->
    <FeatureGate
      feature="changelog"
      title="Breaking Changes"
      description="Know before you break. Breaking changes are detected automatically so you can plan your update safely."
    >
      <div
        v-if="detail.has_breaking_changes"
        class="bg-rose-500/5 rounded-xl p-4 border border-rose-500/20"
      >
        <div class="flex items-center gap-2">
          <AlertTriangle :size="14" class="text-pb-status-down shrink-0" />
          <h4 class="text-xs font-bold text-pb-status-down">Breaking Changes Detected</h4>
        </div>
        <p class="text-[11px] text-rose-300/70 mt-1.5">
          This update contains breaking changes. Review the changelog carefully before proceeding.
        </p>
      </div>
    </FeatureGate>

    <!-- 5. Risk Score (Pro) -->
    <FeatureGate
      feature="risk_scoring"
      title="Risk Score"
      description="Instantly assess the risk of each update. A smart score combines CVE severity, breaking changes, and version jump to help you prioritize."
    >
      <div v-if="detail.risk_score > 0" class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">
          Risk Score
        </h4>
        <RiskScoreGauge :score="detail.risk_score" :level="riskLevel" />
      </div>
    </FeatureGate>

    <!-- 6. Changelog (Pro) -->
    <FeatureGate
      feature="changelog"
      title="Changelog"
      description="Read what changed before you update. Changelogs are fetched automatically with breaking changes highlighted."
    >
      <div v-if="detail.changelog_url || detail.changelog_summary">
        <ChangelogViewer
          :changelog-url="detail.changelog_url"
          :changelog-summary="detail.changelog_summary"
          :has-breaking-changes="detail.has_breaking_changes"
          :source-url="detail.source_url"
        />
      </div>
    </FeatureGate>

    <!-- 7. CVEs (Pro) -->
    <FeatureGate
      feature="cve_enrichment"
      title="Vulnerabilities (CVE)"
      description="See at a glance if your containers are exposed to known vulnerabilities. CVEs are automatically matched and ranked by severity."
    >
      <div class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
        <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-3">
          Vulnerabilities (CVE)
        </h4>
        <CveList :cves="detail.active_cves || []" />
      </div>
    </FeatureGate>

    <!-- 8. Rollback command -->
    <div
      v-if="detail.rollback_command"
      class="bg-pb-primary rounded-xl p-4 border border-amber-900/30"
    >
      <div class="flex items-center justify-between mb-2">
        <h4 class="text-[10px] font-bold text-amber-500/80 uppercase tracking-widest">
          Rollback Command
        </h4>
        <button
          @click="copyRollbackCommand"
          class="text-[10px] text-amber-500 hover:text-amber-400 flex items-center gap-1 transition-colors"
          aria-label="Copy rollback command"
        >
          <component :is="copiedRollback ? Check : Copy" :size="10" />
          {{ copiedRollback ? 'Copied!' : 'Copy' }}
        </button>
      </div>
      <pre
        class="text-[11px] rounded-lg p-3 overflow-x-auto font-mono whitespace-pre" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
        >{{ detail.rollback_command }}</pre
      >
      <p
        v-if="detail.rollback_command.includes('<compose-project-dir>')"
        class="text-[9px] text-slate-600 mt-2"
      >
        Replace &lt;compose-project-dir&gt; with the actual path to your docker-compose.yml
        directory.
      </p>
      <p class="text-[9px] text-slate-600 mt-2">
        Digest availability depends on registry retention policies.
      </p>
    </div>

    <!-- 9. Previous digest (Pro) -->
    <FeatureGate
      feature="cve_enrichment"
      title="Previous Digest"
      description="Keep a rollback safety net. The previous image digest is saved so you can revert in seconds if an update goes wrong."
    >
      <div
        v-if="detail.previous_digest"
        class="rounded-xl p-4 border" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <h4 class="text-[10px] font-bold text-slate-500 uppercase tracking-widest mb-2">
          Previous Digest
        </h4>
        <p class="text-[10px] text-slate-400 font-mono break-all">{{ detail.previous_digest }}</p>
      </div>
    </FeatureGate>

    <!-- Actions -->
    <div class="pt-4 border-t border-slate-800 space-y-3">
      <!-- Pin / Unpin -->
      <div>
        <button
          v-if="!showPinInput || detail.pinned"
          @click="handlePin"
          class="w-full py-2.5 rounded-xl text-xs font-bold transition-all flex items-center justify-center gap-2"
          :class="
            detail.pinned
              ? 'bg-slate-700 hover:bg-slate-600 text-pb-secondary'
              : 'bg-amber-600/20 hover:bg-amber-600/30 text-amber-400 border border-amber-500/20'
          "
        >
          <component :is="detail.pinned ? PinOff : Pin" :size="13" />
          {{ detail.pinned ? 'Unpin this version' : 'Pin this version' }}
        </button>
        <div v-if="showPinInput && !detail.pinned">
          <textarea
            v-model="pinReason"
            rows="2"
            placeholder="Reason (optional)"
            class="w-full px-3 py-2 rounded-lg text-xs placeholder-slate-600 focus:outline-none resize-none"
            style="background: var(--pb-bg-elevated); border: 1px solid var(--pb-border-default); color: var(--pb-text-primary)"
          />
          <button
            @click="handlePin"
            class="mt-2 w-full py-2 bg-amber-600 hover:bg-amber-500 text-slate-950 rounded-lg text-xs font-bold transition-all"
          >
            Confirm pin
          </button>
        </div>
      </div>

      <!-- Source link -->
      <a
        v-if="detail.source_url"
        :href="detail.source_url"
        target="_blank"
        rel="noopener noreferrer"
        class="w-full py-2.5 bg-slate-800 hover:bg-slate-700 text-pb-secondary rounded-xl text-xs font-bold transition-all flex items-center justify-center gap-2"
      >
        <ExternalLink :size="13" />
        View source code
      </a>
    </div>
  </div>

  <div v-else class="text-center py-12">
    <p class="text-sm text-slate-600">No update data available</p>
  </div>
</template>
