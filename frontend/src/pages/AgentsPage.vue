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
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAgentsStore } from '@/stores/agents'
import { useEdition } from '@/composables/useEdition'
import FeatureGate from '@/components/FeatureGate.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import EnrollmentTokenModal from '@/components/EnrollmentTokenModal.vue'
import HostLimitDialog from '@/components/HostLimitDialog.vue'
import AgentDetailPanel from '@/components/AgentDetailPanel.vue'
import { docUrl } from '@/utils/docs'
import { MonitorDot, Server, Boxes, ShieldCheck } from 'lucide-vue-next'
import type { Agent, EnrollmentTokenCreated } from '@/services/agentApi'

const { hasFeature, getQuota } = useEdition()
const store = useAgentsStore()

const isAvailable = computed(() => hasFeature('multihost'))
const hostQuota = getQuota('agent_hosts')

const generatingToken = ref(false)
const tokenModalData = ref<EnrollmentTokenCreated | null>(null)
const tokenError = ref<string | null>(null)
const showLimitDialog = ref(false)

const detailOpen = ref(false)
const selectedAgent = ref<Agent | null>(null)

function openDetail(agent: Agent) {
  selectedAgent.value = agent
  detailOpen.value = true
}

onMounted(() => {
  if (isAvailable.value) {
    store.fetchAgents()
    store.fetchTokens()
    store.fetchMetrics()
    store.connectSSE()
  }
})

onUnmounted(() => {
  store.disconnectSSE()
})

async function handleGenerateToken() {
  // At the host cap, invite a direct conversation instead of enrolling more.
  if (hostQuota.value.isAtLimit) {
    showLimitDialog.value = true
    return
  }
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
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">

      <!-- Header -->
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-mnt-primary">Agents</h1>
          <p class="mt-1 text-sm text-mnt-muted">
            Remote monitoring agents enrolled on this server
          </p>
        </div>
      </div>

      <FeatureHint
        storage-key="agents"
        title="Monitor remote hosts from a single server"
        :doc-href="docUrl('features/multihost/#agent-enrollment')"
      >
        Enroll lightweight agents on remote machines to stream their Docker, Swarm and Kubernetes state back to this server. Generate an enrollment token, run the install command on the host, and the agent appears below.
      </FeatureHint>

      <!-- Pro gate -->
      <FeatureGate feature="multihost">
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
            <button
              :disabled="generatingToken"
              class="min-h-[36px] rounded-lg px-4 text-sm font-medium transition-opacity disabled:opacity-50"
              :style="{ backgroundColor: 'var(--mnt-accent)', color: 'var(--mnt-text-inverted)', borderRadius: 'var(--mnt-radius-md)' }"
              @click="handleGenerateToken"
            >
              {{ generatingToken ? 'Generating…' : 'Generate enrollment token' }}
            </button>
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
                  <p class="text-xs text-mnt-muted font-mono">{{ agent.os_arch }} · v{{ agent.agent_version }}</p>
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
                  <button
                    class="rounded px-2 py-1 text-xs transition-colors revoke-btn"
                    :style="{ color: 'var(--mnt-status-down-text)' }"
                    @click="handleDeleteToken(tok.token_id)"
                  >
                    Revoke
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Placeholder slot (Community Edition) -->
        <template #placeholder>
          <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-mnt-green-500/10 border border-mnt-green-500/20 flex items-center justify-center mb-4">
                <MonitorDot :size="22" class="text-mnt-green-400" />
              </div>
              <h2 class="text-base font-bold text-mnt-primary mb-1">Multi-host Agents</h2>
              <p class="text-sm text-mnt-muted max-w-md mb-6 leading-relaxed">
                Enroll lightweight agents on remote hosts to monitor Docker, Swarm and Kubernetes from this single server — no extra dashboards, no per-host setup.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li class="flex items-start gap-3">
                  <Server :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Monitor unlimited remote hosts from one server with token-based enrollment
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <Boxes :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Auto-detect Docker, Swarm and Kubernetes runtimes on each agent
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <ShieldCheck :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Secure, expiring enrollment tokens you can revoke at any time
                  </span>
                </li>
              </ul>

              <RouterLink
                to="/pro-edition"
                class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-semibold bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted shadow-lg shadow-mnt-green-500/20 transition-colors"
              >
                Unlock with Pro
              </RouterLink>
            </div>
          </div>
        </template>
      </FeatureGate>

    </div>
  </div>

  <!-- Token modal (one-time display) -->
  <EnrollmentTokenModal
    v-if="tokenModalData"
    :token="tokenModalData"
    @close="handleModalClose"
  />

  <!-- Host cap reached — invite a direct conversation -->
  <HostLimitDialog
    v-if="showLimitDialog"
    :used="hostQuota.used"
    :limit="hostQuota.limit"
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
.revoke-btn:hover {
  background-color: var(--mnt-status-down-bg);
}
</style>
