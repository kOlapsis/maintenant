<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'

import { useAgentsStore } from '@/stores/agents'
import { useEdition } from '@/composables/useEdition'
import EnrollmentTokenModal from './EnrollmentTokenModal.vue'
import HostLimitDialog from './HostLimitDialog.vue'
import AgentDetailPanel from './AgentDetailPanel.vue'
import StatusBadge from '@/components/ui/StatusBadge.vue'
import UiTooltip from '@/components/ui/UiTooltip.vue'
import UiButton from '@/components/ui/UiButton.vue'
import { osNeedsAttention, osSupportHint, osSupportLabel, osSupportSeverity } from '@/utils/osSupport'
import type { Agent, EnrollmentTokenCreated } from '@/services/agentApi'
import type { Edition } from '@/services/editionApi'
import { ApiError } from '@/services/apiFetch'

const { getQuota } = useEdition()
const store = useAgentsStore()

const hostQuota = getQuota('agent_hosts')

const generatingToken = ref(false)
const tokenModalData = ref<EnrollmentTokenCreated | null>(null)
const tokenError = ref<string | null>(null)
const showLimitDialog = ref(false)
const limitRefusalEdition = ref<Edition | null>(null)

const detailOpen = ref(false)
const selectedAgent = ref<Agent | null>(null)

function openDetail(agent: Agent) {
  selectedAgent.value = agent
  detailOpen.value = true
}

onMounted(() => {
  store.fetchAgents()
  store.fetchTokens()
  store.fetchMetrics()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})

async function handleGenerateToken() {
  // At the host cap the server refuses anyway; showing the dialog first saves a
  // round trip. The edition that lifts the cap is only known from a refusal, so
  // it stays null on this local shortcut.
  if (hostQuota.value.isAtLimit) {
    limitRefusalEdition.value = null
    showLimitDialog.value = true
    return
  }
  generatingToken.value = true
  tokenError.value = null
  try {
    tokenModalData.value = await store.createToken(24)
  } catch (e) {
    // A server-side HOST_LIMIT_REACHED carries the edition that lifts the cap.
    if (e instanceof ApiError && e.code === 'HOST_LIMIT_REACHED') {
      limitRefusalEdition.value = e.detail?.required_edition ?? null
      showLimitDialog.value = true
      return
    }
    tokenError.value = e instanceof Error ? e.message : 'Failed to generate token'
  } finally {
    generatingToken.value = false
  }
}

function handleModalClose() {
  tokenModalData.value = null
  store.fetchTokens()
}

async function handleDeleteToken(tokenId: string) {
  try {
    await store.deleteToken(tokenId)
  } catch {
    // token may already be consumed — silently refresh
    await store.fetchTokens()
  }
}

function formatDate(dateStr: string | null): string {
  if (!dateStr) return '—'
  return new Date(dateStr).toLocaleString()
}

function runtimeLabel(rt: string): string {
  const map: Record<string, string> = { docker: 'Docker', swarm: 'Swarm', kubernetes: 'K8s' }
  return map[rt] ?? rt
}
</script>

<template>
  <!-- Metrics strip -->
  <div
    v-if="store.metrics"
    class="mb-6 grid grid-cols-2 sm:grid-cols-4 gap-3"
  >
    <div class="rounded-xl border border-mnt-default bg-mnt-surface px-4 py-3">
      <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Total</p>
      <p class="mt-1 text-2xl font-black text-mnt-primary">{{ store.metrics.total }}</p>
    </div>
    <div class="rounded-xl border border-mnt-default bg-mnt-surface px-4 py-3">
      <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Active</p>
      <p class="mt-1 text-2xl font-black text-mnt-primary">{{ store.metrics.by_status.active }}</p>
    </div>
    <div class="rounded-xl border border-mnt-default bg-mnt-surface px-4 py-3">
      <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Connected</p>
      <p class="mt-1 text-2xl font-black text-mnt-primary">{{ store.metrics.by_connection_state.connected }}</p>
    </div>
    <div class="rounded-xl border border-mnt-default bg-mnt-surface px-4 py-3">
      <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Events/s (5m)</p>
      <p class="mt-1 text-2xl font-black text-mnt-primary">{{ store.metrics.total_events_per_second_observed_5m }}</p>
    </div>
  </div>

  <!-- Agents table -->
  <div
    class="overflow-hidden overflow-x-auto rounded-xl border border-mnt-default bg-mnt-surface mb-6"
  >
    <div class="flex items-center justify-between px-4 py-3 border-b border-mnt-subtle">
      <p class="text-sm font-semibold text-mnt-primary">
        Enrolled Agents
        <span
          v-if="hostQuota.limit !== -1"
          class="ml-2 font-mono text-xs"
          :style="{ color: hostQuota.isAtLimit ? 'var(--mnt-status-warn-text)' : 'var(--mnt-text-muted)' }"
        >{{ hostQuota.used }}/{{ hostQuota.limit }}</span>
      </p>
      <UiButton variant="primary" :loading="generatingToken" :disabled="generatingToken" @click="handleGenerateToken">
        {{ generatingToken ? 'Generating…' : 'Generate enrollment token' }}
      </UiButton>
    </div>

    <div v-if="tokenError" class="px-4 py-2 text-xs" :style="{ color: 'var(--mnt-status-down-text)' }">{{ tokenError }}</div>

    <div v-if="store.loading" class="px-4 py-8 text-center text-sm text-mnt-muted">Loading…</div>
    <div v-else-if="store.error" class="px-4 py-4 text-sm" :style="{ color: 'var(--mnt-status-down-text)' }">{{ store.error }}</div>
    <table v-else class="w-full text-sm min-w-[640px]">
      <thead>
        <tr class="border-b border-mnt-subtle">
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Hostname / Label</th>
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Runtime</th>
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Status</th>
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Connection</th>
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest hidden md:table-cell">Last seen</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="agent in store.agents"
          :key="agent.agent_id"
          class="transition-all cursor-pointer group border-b border-mnt-subtle last:border-0 agent-row"
          @click="openDetail(agent)"
        >
          <td class="px-4 py-3">
            <p class="font-medium text-mnt-primary">{{ agent.label || agent.hostname }}</p>
            <p v-if="agent.label && agent.label !== agent.hostname" class="text-xs text-mnt-muted">{{ agent.hostname }}</p>
            <p class="text-xs text-mnt-muted font-mono">{{ agent.os?.pretty_name || 'OS unknown' }} · {{ agent.os_arch }} · v{{ agent.agent_version }}</p>
            <UiTooltip v-if="agent.os && osNeedsAttention(agent.os.support.state)" :text="osSupportHint(agent.os)">
              <StatusBadge
                :severity="osSupportSeverity(agent.os.support.state)"
                :label="osSupportLabel(agent.os.support.state)"
                size="sm"
                show-label
              />
            </UiTooltip>
          </td>
          <td class="px-4 py-3">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{ backgroundColor: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-secondary)' }"
            >{{ runtimeLabel(agent.detected_runtime) }}</span>
          </td>
          <td class="px-4 py-3">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.status === 'active' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-down-bg)',
                color: agent.status === 'active' ? 'var(--mnt-status-ok-text)' : 'var(--mnt-status-down-text)',
              }"
            >{{ agent.status }}</span>
          </td>
          <td class="px-4 py-3">
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.connection_state === 'connected' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-bg-elevated)',
                color: agent.connection_state === 'connected' ? 'var(--mnt-status-ok-text)' : 'var(--mnt-text-muted)',
              }"
            >{{ agent.connection_state ?? 'disconnected' }}</span>
            <span
              v-if="agent.spool?.draining"
              class="ml-1 rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{ backgroundColor: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn-text)' }"
              :title="`${agent.spool.queued} événements en attente de rejeu`"
            >rattrapage · {{ agent.spool.queued }}</span>
            <span
              v-if="agent.spool && agent.spool.dropped_since_connect > 0"
              class="ml-1 rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{ backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down-text)' }"
              :title="'Événements abandonnés faute de place dans le spool de l’agent'"
            >{{ agent.spool.dropped_since_connect }} perdus</span>
          </td>
          <td class="px-4 py-3 text-xs text-mnt-muted hidden md:table-cell">
            {{ formatDate(agent.last_seen_at) }}
          </td>
        </tr>
        <tr v-if="store.agents.length === 0">
          <td colspan="5" class="px-4 py-8 text-center text-sm text-mnt-muted">
            No agents enrolled yet. Generate an enrollment token to get started.
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <!-- Pending tokens -->
  <div
    v-if="store.tokens.length > 0"
    class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface"
  >
    <div class="px-4 py-3 border-b border-mnt-subtle">
      <p class="text-sm font-semibold text-mnt-primary">Pending Enrollment Tokens</p>
      <p class="text-xs text-mnt-muted mt-0.5">Tokens that have not yet been consumed by an agent</p>
    </div>
    <table class="w-full text-sm min-w-[480px]">
      <thead>
        <tr class="border-b border-mnt-subtle">
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Token</th>
          <th class="text-left px-4 py-3 text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Expires</th>
          <th class="px-4 py-3"></th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="tok in store.tokens"
          :key="tok.token_id"
          class="border-b border-mnt-subtle last:border-0"
        >
          <td class="px-4 py-3 font-mono text-xs text-mnt-secondary">{{ tok.token_masked }}</td>
          <td class="px-4 py-3 text-xs text-mnt-muted">{{ formatDate(tok.expires_at) }}</td>
          <td class="px-4 py-3 text-right">
            <UiButton variant="ghost" size="sm" @click="handleDeleteToken(tok.token_id)">Revoke</UiButton>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <!-- Token modal (one-time display) -->
  <EnrollmentTokenModal
    v-if="tokenModalData"
    :token="tokenModalData"
    @close="handleModalClose"
  />

  <!-- Host cap reached: invite a direct conversation -->
  <HostLimitDialog
    v-if="showLimitDialog"
    :used="hostQuota.used"
    :limit="hostQuota.limit"
    :required-edition="limitRefusalEdition"
    @close="showLimitDialog = false"
  />

  <!-- Agent detail panel -->
  <AgentDetailPanel
    v-model:open="detailOpen"
    :agent="selectedAgent"
    @revoked="detailOpen = false"
    @deleted="detailOpen = false"
  />
</template>

<style scoped>
.agent-row:hover {
  background-color: var(--mnt-bg-hover);
}
</style>
