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
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-3">Identity</p>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-mnt-muted">Agent ID</span>
            <span class="font-mono text-xs text-mnt-secondary truncate max-w-[180px]" :title="agent.agent_id">{{ agent.agent_id }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Hostname</span>
            <span class="text-mnt-primary">{{ agent.hostname }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">OS / Arch</span>
            <span class="text-mnt-primary">{{ agent.os_arch }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Version</span>
            <span class="text-mnt-primary">v{{ agent.agent_version }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Runtime</span>
            <span class="text-mnt-primary">{{ runtimeLabel(agent.detected_runtime) }}</span>
          </div>
        </div>
      </div>

      <!-- Label editor -->
      <div>
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Label</p>
        <div class="flex gap-2">
          <input
            v-model="labelDraft"
            maxlength="64"
            placeholder="Display label…"
            class="flex-1 rounded-lg border border-mnt-default bg-mnt-primary px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted focus:outline-none focus:border-mnt-green-500"
            @keydown.enter="saveLabel"
          />
          <button
            :disabled="savingLabel || labelDraft === agent.label"
            class="rounded-lg px-3 py-2 text-sm font-medium transition-opacity disabled:opacity-40"
            :style="{ backgroundColor: 'var(--mnt-accent)', color: 'var(--mnt-text-inverted)' }"
            @click="saveLabel"
          >
            {{ savingLabel ? '…' : 'Save' }}
          </button>
        </div>
        <p v-if="labelConflict" class="mt-1 text-xs text-yellow-500">
          Another agent already uses this label.
        </p>
        <p v-if="labelError" class="mt-1 text-xs text-mnt-status-down">{{ labelError }}</p>
        <p v-if="labelSuccess" class="mt-1 text-xs text-mnt-green-400">Label updated.</p>
      </div>

      <!-- Status -->
      <div>
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-3">Status</p>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between">
            <span class="text-mnt-muted">Status</span>
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.status === 'active' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-down-bg)',
                color: agent.status === 'active' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
              }"
            >{{ agent.status }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Connection</span>
            <span
              class="rounded-full px-2 py-0.5 text-xs font-medium"
              :style="{
                backgroundColor: agent.connection_state === 'connected' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-bg-elevated)',
                color: agent.connection_state === 'connected' ? 'var(--mnt-status-ok)' : 'var(--mnt-text-muted)',
              }"
            >{{ agent.connection_state ?? 'disconnected' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Last seen</span>
            <span class="text-mnt-secondary text-xs">{{ formatDate(agent.last_seen_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-mnt-muted">Enrolled</span>
            <span class="text-mnt-secondary text-xs">{{ formatDate(agent.created_at) }}</span>
          </div>
          <template v-if="agent.revoked_at">
            <div class="flex justify-between">
              <span class="text-mnt-muted">Revoked at</span>
              <span class="text-mnt-status-down text-xs">{{ formatDate(agent.revoked_at) }}</span>
            </div>
            <div class="flex justify-between">
              <span class="text-mnt-muted">Revoked by</span>
              <span class="text-mnt-status-down text-xs">{{ agent.revoked_by ?? '—' }}</span>
            </div>
          </template>
        </div>
      </div>

      <!-- Actions -->
      <div v-if="agent.status === 'active'" class="space-y-2">
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Actions</p>
        <p v-if="actionError" class="text-xs text-mnt-status-down">{{ actionError }}</p>
        <button
          :disabled="revoking"
          class="w-full rounded-lg border border-yellow-600/40 px-4 py-2 text-sm font-medium text-yellow-500 hover:bg-yellow-600/10 transition-colors disabled:opacity-40"
          @click="handleRevoke"
        >
          {{ revoking ? 'Revoking…' : 'Revoke agent' }}
        </button>
        <button
          class="w-full rounded-lg border border-red-600/40 px-4 py-2 text-sm font-medium text-mnt-status-down hover:bg-mnt-sev-incident-solid/10 transition-colors"
          @click="showDeleteConfirm = true"
        >
          Delete agent
        </button>
      </div>

      <div v-if="agent.status === 'revoked'" class="space-y-2">
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Actions</p>
        <p v-if="actionError" class="text-xs text-mnt-status-down">{{ actionError }}</p>
        <button
          class="w-full rounded-lg border border-red-600/40 px-4 py-2 text-sm font-medium text-mnt-status-down hover:bg-mnt-sev-incident-solid/10 transition-colors"
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
        <div class="relative z-10 w-full max-w-md mx-4 rounded-2xl border border-mnt-default bg-mnt-surface p-6 shadow-2xl">
          <h3 class="text-base font-bold text-mnt-primary mb-2">Delete agent?</h3>
          <p class="text-sm text-mnt-muted mb-4">
            This will permanently purge all historical events for this agent
            (containers, endpoints, heartbeats, resources, certificates) in a single transaction.
            <strong class="text-mnt-status-down">This action is irreversible.</strong>
          </p>
          <div class="flex gap-3 justify-end">
            <button
              class="rounded-lg px-4 py-2 text-sm text-mnt-muted hover:text-mnt-primary transition-colors"
              @click="showDeleteConfirm = false"
            >
              Cancel
            </button>
            <button
              :disabled="deleting"
              class="rounded-lg px-4 py-2 text-sm font-medium text-mnt-primary bg-mnt-sev-incident-solid hover:opacity-90 disabled:opacity-40 transition-colors"
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
