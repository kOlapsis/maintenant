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
import { ref, watch, computed } from 'vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import { useAgentsStore } from '@/stores/agents'
import type { Agent } from '@/services/agentApi'

const props = defineProps<{
  open: boolean
  agent: Agent | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  revoked: [agentId: string]
  deleted: [agentId: string]
}>()

const store = useAgentsStore()

const labelDraft = ref('')
const savingLabel = ref(false)
const labelError = ref<string | null>(null)
const labelSuccess = ref(false)

const revoking = ref(false)
const deleting = ref(false)
const showDeleteConfirm = ref(false)
const actionError = ref<string | null>(null)

watch(
  () => props.agent,
  (a) => {
    labelDraft.value = a?.label ?? ''
    labelError.value = null
    labelSuccess.value = false
    actionError.value = null
    showDeleteConfirm.value = false
  },
)

const labelConflict = computed(() => {
  if (!props.agent || !labelDraft.value) return false
  return store.agents.some(
    (a) => a.agent_id !== props.agent!.agent_id && a.label === labelDraft.value,
  )
})

async function saveLabel() {
  if (!props.agent || !labelDraft.value.trim()) return
  if (labelDraft.value.length > 64) {
    labelError.value = 'Label must be ≤ 64 characters'
    return
  }
  savingLabel.value = true
  labelError.value = null
  labelSuccess.value = false
  try {
    await store.updateAgentLabel(props.agent.agent_id, labelDraft.value.trim())
    labelSuccess.value = true
    setTimeout(() => (labelSuccess.value = false), 2000)
  } catch (e) {
    labelError.value = e instanceof Error ? e.message : 'Failed to update label'
  } finally {
    savingLabel.value = false
  }
}

async function handleRevoke() {
  if (!props.agent) return
  revoking.value = true
  actionError.value = null
  try {
    await store.revokeAgent(props.agent.agent_id)
    emit('revoked', props.agent.agent_id)
  } catch (e) {
    actionError.value = e instanceof Error ? e.message : 'Failed to revoke agent'
  } finally {
    revoking.value = false
  }
}

async function handleDelete() {
  if (!props.agent) return
  deleting.value = true
  actionError.value = null
  try {
    await store.deleteAgent(props.agent.agent_id)
    emit('deleted', props.agent.agent_id)
    emit('update:open', false)
  } catch (e) {
    actionError.value = e instanceof Error ? e.message : 'Failed to delete agent'
    deleting.value = false
  }
}

function formatDate(d: string | null): string {
  if (!d) return '—'
  return new Date(d).toLocaleString()
}

function runtimeLabel(rt: string): string {
  return ({ docker: 'Docker', swarm: 'Swarm', kubernetes: 'K8s' })[rt] ?? rt
}
</script>

<template>
  <SlideOverPanel :open="open" title="Agent details" width="max-w-lg" @update:open="$emit('update:open', $event)">
    <div v-if="agent" class="px-5 py-4 space-y-6 overflow-y-auto">

      <!-- Identity -->
      <div>
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-3">Identity</p>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-slate-500">Agent ID</span>
            <span class="font-mono text-xs text-slate-300 truncate max-w-[180px]" :title="agent.agent_id">{{ agent.agent_id }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Hostname</span>
            <span class="text-white">{{ agent.hostname }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">OS / Arch</span>
            <span class="text-white">{{ agent.os_arch }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Version</span>
            <span class="text-white">v{{ agent.agent_version }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Runtime</span>
            <span class="text-white">{{ runtimeLabel(agent.detected_runtime) }}</span>
          </div>
        </div>
      </div>

      <!-- Label editor -->
      <div>
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Label</p>
        <div class="flex gap-2">
          <input
            v-model="labelDraft"
            maxlength="64"
            placeholder="Display label…"
            class="flex-1 rounded-lg border border-slate-700 bg-[#0B0E13] px-3 py-2 text-sm text-white placeholder-slate-600 focus:outline-none focus:border-pb-green-500"
            @keydown.enter="saveLabel"
          />
          <button
            :disabled="savingLabel || labelDraft === agent.label"
            class="rounded-lg px-3 py-2 text-sm font-medium transition-opacity disabled:opacity-40"
            :style="{ backgroundColor: 'var(--pb-accent)', color: 'var(--pb-text-inverted)' }"
            @click="saveLabel"
          >
            {{ savingLabel ? '…' : 'Save' }}
          </button>
        </div>
        <p v-if="labelConflict" class="mt-1 text-xs text-yellow-500">
          Another agent already uses this label.
        </p>
        <p v-if="labelError" class="mt-1 text-xs text-red-400">{{ labelError }}</p>
        <p v-if="labelSuccess" class="mt-1 text-xs text-pb-green-400">Label updated.</p>
      </div>

      <!-- Status -->
      <div>
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-3">Status</p>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-slate-500">Status</span>
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.status === 'active' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
                color: agent.status === 'active' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
              }"
            >{{ agent.status }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Connection</span>
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.connection_state === 'connected' ? 'var(--pb-status-ok-bg)' : 'var(--pb-bg-elevated)',
                color: agent.connection_state === 'connected' ? 'var(--pb-status-ok)' : 'var(--pb-text-muted)',
              }"
            >{{ agent.connection_state ?? 'disconnected' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Last seen</span>
            <span class="text-slate-300 text-xs">{{ formatDate(agent.last_seen_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-slate-500">Enrolled</span>
            <span class="text-slate-300 text-xs">{{ formatDate(agent.created_at) }}</span>
          </div>
          <template v-if="agent.revoked_at">
            <div class="flex justify-between">
              <span class="text-slate-500">Revoked at</span>
              <span class="text-red-400 text-xs">{{ formatDate(agent.revoked_at) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-slate-500">Revoked by</span>
              <span class="text-red-400 text-xs">{{ agent.revoked_by ?? '—' }}</span>
            </div>
          </template>
        </div>
      </div>

      <!-- Actions -->
      <div v-if="agent.status === 'active'" class="space-y-2">
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Actions</p>
        <p v-if="actionError" class="text-xs text-red-400">{{ actionError }}</p>
        <button
          :disabled="revoking"
          class="w-full rounded-lg border border-yellow-600/40 px-4 py-2 text-sm font-medium text-yellow-500 hover:bg-yellow-600/10 transition-colors disabled:opacity-40"
          @click="handleRevoke"
        >
          {{ revoking ? 'Revoking…' : 'Revoke agent' }}
        </button>
        <button
          class="w-full rounded-lg border border-red-600/40 px-4 py-2 text-sm font-medium text-red-500 hover:bg-red-600/10 transition-colors"
          @click="showDeleteConfirm = true"
        >
          Delete agent
        </button>
      </div>

      <div v-if="agent.status === 'revoked'" class="space-y-2">
        <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-2">Actions</p>
        <p v-if="actionError" class="text-xs text-red-400">{{ actionError }}</p>
        <button
          class="w-full rounded-lg border border-red-600/40 px-4 py-2 text-sm font-medium text-red-500 hover:bg-red-600/10 transition-colors"
          @click="showDeleteConfirm = true"
        >
          Delete agent
        </button>
      </div>
    </div>

    <!-- Delete confirmation modal -->
    <Teleport to="body">
      <div
        v-if="showDeleteConfirm"
        class="fixed inset-0 z-[10000] flex items-center justify-center"
        @click.self="showDeleteConfirm = false"
      >
        <div class="absolute inset-0 bg-black/70" @click="showDeleteConfirm = false" />
        <div class="relative z-10 w-full max-w-md mx-4 rounded-2xl border border-slate-700 bg-[#12151C] p-6 shadow-2xl">
          <h3 class="text-base font-bold text-white mb-2">Delete agent?</h3>
          <p class="text-sm text-slate-400 mb-4">
            This will permanently purge all historical events for this agent
            (containers, endpoints, heartbeats, resources, certificates) in a single transaction.
            <strong class="text-red-400">This action is irreversible.</strong>
          </p>
          <div class="flex gap-3 justify-end">
            <button
              class="rounded-lg px-4 py-2 text-sm text-slate-400 hover:text-white transition-colors"
              @click="showDeleteConfirm = false"
            >
              Cancel
            </button>
            <button
              :disabled="deleting"
              class="rounded-lg px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 disabled:opacity-40 transition-colors"
              @click="handleDelete"
            >
              {{ deleting ? 'Deleting…' : 'Delete permanently' }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </SlideOverPanel>
</template>
