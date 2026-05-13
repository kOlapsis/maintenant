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
import { onMounted, onUnmounted, ref } from 'vue'
import { useAgentsStore } from '@/stores/agents'
import FeatureGate from '@/components/FeatureGate.vue'
import EnrollmentTokenModal from '@/components/EnrollmentTokenModal.vue'
import AgentDetailPanel from '@/components/AgentDetailPanel.vue'
import type { Agent, EnrollmentTokenCreated } from '@/services/agentApi'

const store = useAgentsStore()

const generatingToken = ref(false)
const tokenModalData = ref<EnrollmentTokenCreated | null>(null)
const tokenError = ref<string | null>(null)

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
  generatingToken.value = true
  tokenError.value = null
  try {
    tokenModalData.value = await store.createToken(24)
  } catch (e) {
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
  } catch (e) {
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
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">

      <!-- Header -->
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-pb-primary">Agents</h1>
          <p class="mt-1 text-sm text-pb-muted">
            Remote monitoring agents enrolled on this server
          </p>
        </div>
      </div>

      <FeatureGate
        feature="multihost"
        title="Multi-host Agents"
        description="Enroll remote agents to monitor Docker, Swarm and Kubernetes hosts from a single server."
      >
        <!-- Metrics strip -->
        <div
          v-if="store.metrics"
          class="mb-6 grid grid-cols-2 sm:grid-cols-4 gap-3"
        >
          <div class="rounded-xl border border-pb-default bg-pb-surface px-4 py-3">
            <p class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Total</p>
            <p class="mt-1 text-2xl font-black text-pb-primary">{{ store.metrics.total }}</p>
          </div>
          <div class="rounded-xl border border-pb-default bg-pb-surface px-4 py-3">
            <p class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Active</p>
            <p class="mt-1 text-2xl font-black text-pb-primary">{{ store.metrics.by_status.active }}</p>
          </div>
          <div class="rounded-xl border border-pb-default bg-pb-surface px-4 py-3">
            <p class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Connected</p>
            <p class="mt-1 text-2xl font-black text-pb-primary">{{ store.metrics.by_connection_state.connected }}</p>
          </div>
          <div class="rounded-xl border border-pb-default bg-pb-surface px-4 py-3">
            <p class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Events/s (5m)</p>
            <p class="mt-1 text-2xl font-black text-pb-primary">{{ store.metrics.total_events_per_second_observed_5m }}</p>
          </div>
        </div>

        <!-- Agents table -->
        <div
          class="overflow-hidden overflow-x-auto rounded-xl border border-pb-default bg-pb-surface mb-6"
        >
          <div class="flex items-center justify-between px-4 py-3 border-b border-pb-subtle">
            <p class="text-sm font-semibold text-pb-primary">Enrolled Agents</p>
            <button
              :disabled="generatingToken"
              class="min-h-[36px] rounded-lg px-4 text-sm font-medium transition-opacity disabled:opacity-50"
              :style="{ backgroundColor: 'var(--pb-accent)', color: 'var(--pb-text-inverted)', borderRadius: 'var(--pb-radius-md)' }"
              @click="handleGenerateToken"
            >
              {{ generatingToken ? 'Generating…' : 'Generate enrollment token' }}
            </button>
          </div>

          <div v-if="tokenError" class="px-4 py-2 text-xs" :style="{ color: 'var(--pb-status-down-text)' }">{{ tokenError }}</div>

          <div v-if="store.loading" class="px-4 py-8 text-center text-sm text-pb-muted">Loading…</div>
          <div v-else-if="store.error" class="px-4 py-4 text-sm" :style="{ color: 'var(--pb-status-down-text)' }">{{ store.error }}</div>
          <table v-else class="w-full text-sm min-w-[640px]">
            <thead>
              <tr class="border-b border-pb-subtle">
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Hostname / Label</th>
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Runtime</th>
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Status</th>
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Connection</th>
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest hidden md:table-cell">Last seen</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="agent in store.agents"
                :key="agent.agent_id"
                class="transition-all cursor-pointer group border-b border-pb-subtle last:border-0 agent-row"
                @click="openDetail(agent)"
              >
                <td class="px-4 py-3">
                  <p class="font-medium text-pb-primary">{{ agent.label || agent.hostname }}</p>
                  <p v-if="agent.label && agent.label !== agent.hostname" class="text-xs text-pb-muted">{{ agent.hostname }}</p>
                  <p class="text-xs text-pb-muted font-mono">{{ agent.os_arch }} · v{{ agent.agent_version }}</p>
                </td>
                <td class="px-4 py-3">
                  <span
                    class="rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{ backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-secondary)' }"
                  >{{ runtimeLabel(agent.detected_runtime) }}</span>
                </td>
                <td class="px-4 py-3">
                  <span
                    class="rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{
                      backgroundColor: agent.status === 'active' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
                      color: agent.status === 'active' ? 'var(--pb-status-ok-text)' : 'var(--pb-status-down-text)',
                    }"
                  >{{ agent.status }}</span>
                </td>
                <td class="px-4 py-3">
                  <span
                    class="rounded-full px-2 py-0.5 text-xs font-medium"
                    :style="{
                      backgroundColor: agent.connection_state === 'connected' ? 'var(--pb-status-ok-bg)' : 'var(--pb-bg-elevated)',
                      color: agent.connection_state === 'connected' ? 'var(--pb-status-ok-text)' : 'var(--pb-text-muted)',
                    }"
                  >{{ agent.connection_state ?? 'disconnected' }}</span>
                </td>
                <td class="px-4 py-3 text-xs text-pb-muted hidden md:table-cell">
                  {{ formatDate(agent.last_seen_at) }}
                </td>
              </tr>
              <tr v-if="store.agents.length === 0">
                <td colspan="5" class="px-4 py-8 text-center text-sm text-pb-muted">
                  No agents enrolled yet. Generate an enrollment token to get started.
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Pending tokens -->
        <div
          v-if="store.tokens.length > 0"
          class="overflow-hidden rounded-xl border border-pb-default bg-pb-surface"
        >
          <div class="px-4 py-3 border-b border-pb-subtle">
            <p class="text-sm font-semibold text-pb-primary">Pending Enrollment Tokens</p>
            <p class="text-xs text-pb-muted mt-0.5">Tokens that have not yet been consumed by an agent</p>
          </div>
          <table class="w-full text-sm min-w-[480px]">
            <thead>
              <tr class="border-b border-pb-subtle">
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Token</th>
                <th class="text-left px-4 py-3 text-[10px] text-pb-muted font-bold uppercase tracking-widest">Expires</th>
                <th class="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="tok in store.tokens"
                :key="tok.token_id"
                class="border-b border-pb-subtle last:border-0"
              >
                <td class="px-4 py-3 font-mono text-xs text-pb-secondary">{{ tok.token_masked }}</td>
                <td class="px-4 py-3 text-xs text-pb-muted">{{ formatDate(tok.expires_at) }}</td>
                <td class="px-4 py-3 text-right">
                  <button
                    class="rounded px-2 py-1 text-xs transition-colors revoke-btn"
                    :style="{ color: 'var(--pb-status-down-text)' }"
                    @click="handleDeleteToken(tok.token_id)"
                  >
                    Revoke
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </FeatureGate>

    </div>
  </div>

  <!-- Token modal (one-time display) -->
  <EnrollmentTokenModal
    v-if="tokenModalData"
    :token="tokenModalData"
    @close="handleModalClose"
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
  background-color: var(--pb-bg-hover);
}
.revoke-btn:hover {
  background-color: var(--pb-status-down-bg);
}
</style>
