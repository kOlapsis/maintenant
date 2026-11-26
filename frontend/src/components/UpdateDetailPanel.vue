<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useUpdatesStore } from '@/stores/updates'
import { useAlertsStore } from '@/stores/alerts'
import { useAgentsStore } from '@/stores/agents'
import { isLocalAgent } from '@/services/apiFetch'
import AgentBadge from '@/components/AgentBadge.vue'
import { type ContainerUpdateDetail, fetchContainerUpdate } from '@/services/updateApi'
import { type Alert } from '@/services/alertApi'
import { useEdition } from '@/composables/useEdition'
import RiskScoreGauge from '@/commercial/components/updates/RiskScoreGauge.vue'
import CveList from '@/commercial/components/updates/CveList.vue'
import ChangelogViewer from '@/commercial/components/updates/ChangelogViewer.vue'
import FeatureGate from '@/components/FeatureGate.vue'
import AcknowledgeButton from '@/components/ui/AcknowledgeButton.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { copyText } from '@/utils/copyText'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import TextareaInput from '@/components/ui/TextareaInput.vue'
import { osCycleText, osStateTextClass } from '@/utils/osSupport'
import { AlertTriangle, ArrowRight, Check, Copy, ExternalLink, Pin, PinOff } from 'lucide-vue-next'

const { hasFeature } = useEdition()

const props = defineProps<{
  containerId: string
  agentId?: string | null
}>()

const updates = useUpdatesStore()
const alertsStore = useAlertsStore()
const agents = useAgentsStore()
const detail = ref<ContainerUpdateDetail | null>(null)
const loading = ref(true)
const error = ref('')
const pinning = ref(false)
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

const unorderedTag = computed(() =>
  detail.value?.update_type === 'digest_only' && !/^v?\d/.test(detail.value.current_tag))

// Alerte update active de ce conteneur : même nom et même hôte (deux hôtes peuvent porter le même nom).
const updateAlert = computed<Alert | undefined>(() => {
  const d = detail.value
  const name = d?.container_name
  if (!d || !name) return undefined
  const all = [
    ...alertsStore.activeAlerts.critical,
    ...alertsStore.activeAlerts.warning,
    ...alertsStore.activeAlerts.info,
  ]
  return all.find((a) => a.source === 'update' && a.entity_name === name &&
    (isLocalAgent(a.agent_id) ? isLocalAgent(d.agent_id) : a.agent_id === d.agent_id))
})

async function loadDetail() {
  loading.value = true
  error.value = ''
  try {
    detail.value = await fetchContainerUpdate(props.containerId, props.agentId)
    if (hasFeature('cve_enrichment')) {
      await updates.fetchContainerCves(props.containerId, props.agentId)
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load update details'
  } finally {
    loading.value = false
  }
}

async function copyCommand() {
  if (!detail.value?.update_command) return
  if (!(await copyText(detail.value.update_command))) return
  copied.value = true
  setTimeout(() => {
    copied.value = false
  }, 2000)
}

async function copyRollbackCommand() {
  if (!detail.value?.rollback_command) return
  if (!(await copyText(detail.value.rollback_command))) return
  copiedRollback.value = true
  setTimeout(() => {
    copiedRollback.value = false
  }, 2000)
}

async function handlePin() {
  if (pinning.value) return
  if (!detail.value?.pinned && !showPinInput.value) {
    showPinInput.value = true
    return
  }
  pinning.value = true
  error.value = ''
  try {
    if (detail.value?.pinned) {
      await updates.unpinVersion(props.containerId, props.agentId)
    } else {
      await updates.pinVersion(props.containerId, pinReason.value, props.agentId)
      showPinInput.value = false
      pinReason.value = ''
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to update the pin'
    return
  } finally {
    pinning.value = false
  }
  await loadDetail()
}

onMounted(loadDetail)
</script>

<template>
  <div v-if="loading" class="flex items-center justify-center py-12">
    <div
      class="w-6 h-6 border-2 border-mnt-green-500 border-t-transparent rounded-full animate-spin"
    />
  </div>

  <div v-else-if="detail" class="space-y-5">
    <!-- 1. Version info -->
    <div class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <div class="mb-3 flex items-center justify-between gap-2">
        <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">Version</h4>
        <AgentBadge v-if="agents.hasRemoteAgents" :agent-id="detail.agent_id" :hostname="detail.agent_hostname" :label="detail.agent_label" show-local />
        <AcknowledgeButton v-if="updateAlert" :alert="updateAlert" />
      </div>
      <div class="flex items-center gap-3">
        <div class="text-center">
          <p class="text-xs text-mnt-muted mb-0.5">Current</p>
          <p class="text-sm font-bold text-mnt-primary font-mono">
            {{ detail.current_tag || 'latest' }}
          </p>
        </div>
        <ArrowRight :size="16" class="text-mnt-green-500 shrink-0" />
        <div class="text-center">
          <p class="text-xs text-mnt-muted mb-0.5">Available</p>
          <p class="text-sm font-bold text-mnt-green-400 font-mono">{{ detail.latest_tag }}</p>
        </div>
      </div>
      <p v-if="detail.major_tag" class="mt-2 text-xs text-mnt-muted break-all" data-testid="update-major">
        Next major: <span class="font-mono">{{ detail.major_tag }}</span>
      </p>
      <p
        v-if="detail.support"
        class="mt-1 text-xs font-bold"
        :class="osStateTextClass(detail.support.state)"
        data-testid="update-support"
      >{{ osCycleText(detail.support) }}</p>
      <div class="mt-2 flex items-center gap-2">
        <span
          class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded bg-mnt-sev-neutral-solid/10 text-mnt-muted"
          >{{ detail.update_type }}</span
        >
        <span
          v-if="detail.pinned"
          class="text-[9px] font-bold uppercase px-1.5 py-0.5 rounded bg-mnt-sev-neutral-solid/10 text-mnt-muted"
        >
          <Pin :size="8" class="inline mr-0.5" /> Pinned
        </span>
      </div>
      <p v-if="unorderedTag" class="mt-2 text-xs text-mnt-muted break-all" data-testid="update-unordered-tag">
        <span class="font-mono">{{ detail.current_tag }}</span> is not a version number: a newer version is never looked up, only a new image published under this tag is reported.
      </p>
      <p v-if="detail.pinned && detail.pin_reason" class="mt-2 text-xs text-mnt-muted italic">
        {{ detail.pin_reason }}
      </p>
    </div>

    <!-- 2. Tag Filter (when configured) -->
    <div
      v-if="detail.tag_include || detail.tag_exclude"
      class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
    >
      <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest mb-3">
        Tag Filter
      </h4>
      <div class="space-y-2">
        <div v-if="detail.tag_include" class="flex items-start gap-2">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest shrink-0 pt-0.5 w-14">Include</span>
          <code class="text-[11px] text-mnt-green-400 font-mono bg-mnt-green-500/5 px-2 py-0.5 rounded break-all">{{ detail.tag_include }}</code>
        </div>
        <div v-if="detail.tag_exclude" class="flex items-start gap-2">
          <span class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest shrink-0 pt-0.5 w-14">Exclude</span>
          <code class="text-[11px] text-mnt-status-down font-mono bg-mnt-status-down px-2 py-0.5 rounded break-all">{{ detail.tag_exclude }}</code>
        </div>
      </div>
    </div>

    <!-- 3. Update command -->
    <div v-if="detail.update_command" class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
      <div class="flex items-center justify-between mb-2">
        <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest">
          Update Command
        </h4>
        <UiButton
          variant="ghost"
          size="sm"
          :icon="copied ? Check : Copy"
          class="!min-h-0 px-1.5 py-0 text-[10px] text-mnt-green-500 hover:text-mnt-green-400"
          aria-label="Copy update command"
          @click="copyCommand"
        >
          {{ copied ? 'Copied!' : 'Copy' }}
        </UiButton>
      </div>
      <pre
        class="text-[11px] rounded-lg p-3 overflow-x-auto font-mono whitespace-pre" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
        >{{ detail.update_command }}</pre
      >
      <p
        v-if="detail.update_command.includes('<compose-project-dir>')"
        class="text-[9px] text-mnt-muted mt-2"
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
        class="bg-mnt-status-down rounded-xl p-4 border border-rose-500/20"
      >
        <div class="flex items-center gap-2">
          <AlertTriangle :size="14" class="text-mnt-status-down shrink-0" />
          <h4 class="text-xs font-bold text-mnt-status-down">Breaking Changes Detected</h4>
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
      description="See how urgent each update is. The score combines the CVEs the update fixes, the support status of the running version and the update type to help you prioritize."
    >
      <div v-if="detail.risk_score > 0" class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
        <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest mb-3">
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
      <div class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)">
        <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest mb-3">
          Vulnerabilities (CVE)
        </h4>
        <CveList :cves="detail.active_cves || []" />
      </div>
    </FeatureGate>

    <!-- 8. Rollback command -->
    <div
      v-if="detail.rollback_command"
      class="bg-mnt-primary rounded-xl p-4 border border-amber-900/30"
    >
      <div class="flex items-center justify-between mb-2">
        <h4 class="text-[10px] font-bold text-mnt-status-warn/80 uppercase tracking-widest">
          Rollback Command
        </h4>
        <UiButton
          variant="ghost"
          size="sm"
          :icon="copiedRollback ? Check : Copy"
          class="!min-h-0 px-1.5 py-0 text-[10px] text-mnt-status-warn"
          aria-label="Copy rollback command"
          @click="copyRollbackCommand"
        >
          {{ copiedRollback ? 'Copied!' : 'Copy' }}
        </UiButton>
      </div>
      <pre
        class="text-[11px] rounded-lg p-3 overflow-x-auto font-mono whitespace-pre" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)"
        >{{ detail.rollback_command }}</pre
      >
      <p
        v-if="detail.rollback_command.includes('<compose-project-dir>')"
        class="text-[9px] text-mnt-muted mt-2"
      >
        Replace &lt;compose-project-dir&gt; with the actual path to your docker-compose.yml
        directory.
      </p>
      <p class="text-[9px] text-mnt-muted mt-2">
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
        class="rounded-xl p-4 border" style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
      >
        <h4 class="text-[10px] font-bold text-mnt-muted uppercase tracking-widest mb-2">
          Previous Digest
        </h4>
        <p class="text-[10px] text-mnt-muted font-mono break-all">{{ detail.previous_digest }}</p>
      </div>
    </FeatureGate>

    <!-- Actions -->
    <div class="pt-4 border-t border-mnt-default space-y-3">
      <InlineAlert v-if="error" severity="critical" title="Request failed" tag="">{{ error }}</InlineAlert>

      <!-- Pin / Unpin -->
      <div>
        <UiButton
          v-if="!showPinInput || detail.pinned"
          :variant="detail.pinned ? 'secondary' : 'primary'"
          class="w-full"
          :loading="pinning"
          :icon="detail.pinned ? PinOff : Pin"
          @click="handlePin"
        >
          {{ detail.pinned ? 'Unpin this version' : 'Pin this version' }}
        </UiButton>
        <div v-if="showPinInput && !detail.pinned">
          <TextareaInput v-model="pinReason" :rows="2" placeholder="Reason (optional)" />
          <UiButton variant="primary" class="mt-2 w-full" :loading="pinning" @click="handlePin">
            Confirm pin
          </UiButton>
        </div>
      </div>

      <!-- Source link -->
      <a
        v-if="detail.source_url"
        :href="detail.source_url"
        target="_blank"
        rel="noopener noreferrer"
        class="w-full py-2.5 bg-mnt-elevated hover:bg-mnt-elevated text-mnt-secondary rounded-xl text-xs font-bold transition-all flex items-center justify-center gap-2"
      >
        <ExternalLink :size="13" />
        View source code
      </a>
    </div>
  </div>

  <InlineAlert v-else-if="error" severity="critical" title="Update details unavailable" tag="">{{ error }}</InlineAlert>

  <div v-else class="text-center py-12">
    <p class="text-sm text-mnt-muted">No update data available</p>
  </div>
</template>
