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
import StatusBadge from '@/components/ui/StatusBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import TextInput from '@/components/ui/TextInput.vue'
import { useAgentsStore } from '@/stores/agents'
import { useConfirm } from '@/composables/useConfirm'
import { osDaysText, osSupportHint, osSupportLabel, osSupportSeverity } from '@/utils/osSupport'
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
const confirm = useConfirm()

const labelDraft = ref('')
const savingLabel = ref(false)
const labelError = ref<string | null>(null)
const labelSuccess = ref(false)

const revoking = ref(false)
const deleting = ref(false)
const actionError = ref<string | null>(null)

watch(
  () => props.agent,
  (a) => {
    labelDraft.value = a?.label ?? ''
    labelError.value = null
    labelSuccess.value = false
    actionError.value = null
  },
)

const osDays = computed(() => (props.agent?.os ? osDaysText(props.agent.os.support) : ''))

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
  const ok = await confirm({
    title: 'Delete agent?',
    message:
      'This will permanently purge all historical events for this agent (containers, endpoints, heartbeats, resources, certificates) in a single transaction. This action is irreversible.',
    confirmLabel: 'Delete permanently',
    destructive: true,
  })
  if (!ok) return
  deleting.value = true
  actionError.value = null
  try {
    await store.deleteAgent(props.agent.agent_id)
    emit('deleted', props.agent.agent_id)
    emit('update:open', false)
  } catch (e) {
    actionError.value = e instanceof Error ? e.message : 'Failed to delete agent'
  } finally {
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

      <!-- Operating system -->
      <div v-if="agent.os">
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-3">Operating system</p>
        <div class="space-y-2 text-sm">
          <div class="flex justify-between gap-3">
            <span class="text-mnt-muted">Distribution</span>
            <span class="text-mnt-primary text-right">{{ agent.os.pretty_name || 'Unknown' }}</span>
          </div>
          <div class="flex justify-between items-center gap-3">
            <span class="text-mnt-muted">Support</span>
            <StatusBadge
              :severity="osSupportSeverity(agent.os.support.state)"
              :label="osSupportLabel(agent.os.support.state)"
              size="sm"
              show-label
            />
          </div>
          <div v-if="agent.os.support.active_until" class="flex justify-between gap-3">
            <span class="text-mnt-muted">Active support until</span>
            <span class="text-mnt-secondary text-xs">{{ agent.os.support.active_until }}</span>
          </div>
          <div v-if="agent.os.support.security_until" class="flex justify-between gap-3">
            <span class="text-mnt-muted">Free security support until</span>
            <span class="text-mnt-secondary text-xs">{{ agent.os.support.security_until }}</span>
          </div>
          <div v-if="agent.os.support.extended_until" class="flex justify-between gap-3">
            <span class="text-mnt-muted">Paid extended support until</span>
            <span class="text-mnt-secondary text-xs">{{ agent.os.support.extended_until }}</span>
          </div>
          <div v-if="osDays" class="flex justify-between gap-3">
            <span class="text-mnt-muted">Remaining</span>
            <span class="text-mnt-secondary text-xs">{{ osDays }}</span>
          </div>
          <div v-if="agent.os.support.table_source" class="flex justify-between gap-3">
            <span class="text-mnt-muted">Source</span>
            <span class="text-mnt-secondary text-xs">{{ agent.os.support.table_source }}</span>
          </div>
          <p class="text-xs text-mnt-muted">{{ osSupportHint(agent.os) }}</p>
        </div>
      </div>

      <!-- Label editor -->
      <div>
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Label</p>
        <div class="flex gap-2">
          <TextInput
            v-model="labelDraft"
            maxlength="64"
            placeholder="Display label…"
            class="flex-1"
            @keydown.enter="saveLabel"
          />
          <UiButton variant="primary" :disabled="savingLabel || labelDraft === agent.label" @click="saveLabel">
            {{ savingLabel ? '…' : 'Save' }}
          </UiButton>
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
        <UiButton variant="secondary" class="w-full" :loading="revoking" :disabled="revoking" @click="handleRevoke">
          {{ revoking ? 'Revoking…' : 'Revoke agent' }}
        </UiButton>
        <UiButton variant="danger" class="w-full" :loading="deleting" :disabled="deleting" @click="handleDelete">
          {{ deleting ? 'Deleting…' : 'Delete agent' }}
        </UiButton>
      </div>

      <div v-if="agent.status === 'revoked'" class="space-y-2">
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Actions</p>
        <p v-if="actionError" class="text-xs text-mnt-status-down">{{ actionError }}</p>
        <UiButton variant="danger" class="w-full" :loading="deleting" :disabled="deleting" @click="handleDelete">
          {{ deleting ? 'Deleting…' : 'Delete agent' }}
        </UiButton>
      </div>
    </div>
  </SlideOverPanel>
</template>
