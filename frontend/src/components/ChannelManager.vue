<script setup lang="ts">
import { ref } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import {
  createChannel,
  updateChannel,
  deleteChannel,
  testChannel,
  createRoutingRule,
  deleteRoutingRule,
} from '@/services/alertApi'
import ChannelWizard from '@/components/ChannelWizard.vue'

const store = useAlertsStore()

const showForm = ref(false)
const showWizard = ref(false)
const editingId = ref<number | null>(null)
const form = ref({ name: '', url: '', headers: '', enabled: true })
const testResult = ref<{ id: number; status: string; response_code?: number; error?: string } | null>(null)

// Routing rule form
const showRuleForm = ref<number | null>(null)
const ruleForm = ref({ source_filter: '', severity_filter: '' })

function resetForm() {
  form.value = { name: '', url: '', headers: '', enabled: true }
  editingId.value = null
  showForm.value = false
}

function startEdit(ch: { id: number; name: string; url: string; headers: string; enabled: boolean }) {
  editingId.value = ch.id
  form.value = { name: ch.name, url: ch.url, headers: ch.headers, enabled: ch.enabled }
  showForm.value = true
  showWizard.value = false
}

async function submitForm() {
  if (editingId.value) {
    await updateChannel(editingId.value, form.value)
  } else {
    await createChannel(form.value)
  }
  resetForm()
  store.fetchChannels()
}

async function handleDelete(id: number) {
  if (!confirm('Delete this channel?')) return
  await deleteChannel(id)
  store.fetchChannels()
}

async function handleTest(id: number) {
  testResult.value = null
  const res = await testChannel(id)
  testResult.value = { id, ...res }
}

async function handleAddRule(channelId: number) {
  await createRoutingRule(channelId, ruleForm.value)
  ruleForm.value = { source_filter: '', severity_filter: '' }
  showRuleForm.value = null
  store.fetchChannels()
}

async function handleDeleteRule(channelId: number, ruleId: number) {
  await deleteRoutingRule(channelId, ruleId)
  store.fetchChannels()
}

function maskUrl(url: string): string {
  try {
    const u = new URL(url)
    const path = u.pathname
    return `${u.protocol}//${u.host}${path.length > 20 ? path.slice(0, 20) + '...' : path}`
  } catch {
    return url.slice(0, 30) + '...'
  }
}

function handleWizardCreated(id: number) {
  showWizard.value = false
  store.fetchChannels()
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold" style="color: var(--pb-text-primary)">Notification Channels</h2>
      <div class="flex gap-2">
        <button
          @click="showWizard = true; showForm = false"
          class="rounded-md px-3 py-1.5 text-sm font-medium text-white"
          style="background: var(--pb-accent)"
        >
          Add Channel
        </button>
      </div>
    </div>

    <!-- Channel Wizard -->
    <div v-if="showWizard" class="mb-4">
      <ChannelWizard
        @created="handleWizardCreated"
        @cancel="showWizard = false"
      />
    </div>

    <!-- Edit form (for existing channels) -->
    <div v-if="showForm && editingId" class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)">
      <h3 class="mb-3 text-sm font-medium" style="color: var(--pb-text-primary)">Edit Channel</h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Name</label>
          <input v-model="form.name" required class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Webhook URL</label>
          <input v-model="form.url" required type="url" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Custom Headers (JSON)</label>
          <input v-model="form.headers" placeholder='{"Authorization": "Bearer ..."}' class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" id="ch-enabled" class="rounded" style="accent-color: var(--pb-accent)" />
          <label for="ch-enabled" class="text-sm" style="color: var(--pb-text-secondary)">Enabled</label>
        </div>
        <div class="flex gap-2">
          <button type="submit" class="rounded-md px-3 py-1.5 text-sm text-white" style="background: var(--pb-accent)">Save</button>
          <button type="button" @click="resetForm" class="rounded-md border px-3 py-1.5 text-sm" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Cancel</button>
        </div>
      </form>
    </div>

    <!-- Channel list -->
    <div class="space-y-3">
      <div
        v-if="store.channels.length === 0 && !store.channelsLoading"
        class="rounded-lg border p-6 text-center"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <p class="text-sm" style="color: var(--pb-text-muted)">No notification channels configured</p>
      </div>

      <div
        v-for="ch in store.channels"
        :key="ch.id"
        class="rounded-lg border p-4"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ background: ch.health === 'healthy' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)' }"
            ></span>
            <div>
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ ch.name }}</span>
                <span v-if="!ch.enabled" class="rounded px-1.5 py-0.5 text-xs" style="background: var(--pb-bg-elevated); color: var(--pb-text-muted)">disabled</span>
              </div>
              <p class="text-xs" style="color: var(--pb-text-muted)">{{ maskUrl(ch.url) }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button @click="handleTest(ch.id)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Test</button>
            <button @click="startEdit(ch)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)">Edit</button>
            <button @click="handleDelete(ch.id)" class="rounded border px-2 py-1 text-xs" style="border-color: var(--pb-status-down); color: var(--pb-status-down)">Delete</button>
          </div>
        </div>

        <!-- Test result -->
        <div
          v-if="testResult && testResult.id === ch.id"
          class="mt-2 rounded border px-3 py-1.5 text-xs"
          :style="{
            background: testResult.status === 'delivered' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
            borderColor: testResult.status === 'delivered' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
            color: testResult.status === 'delivered' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
          }"
        >
          {{ testResult.status === 'delivered' ? `Delivered (HTTP ${testResult.response_code})` : `Failed: ${testResult.error}` }}
        </div>

        <!-- Routing rules -->
        <div v-if="ch.routing_rules && ch.routing_rules.length > 0" class="mt-3 border-t pt-2" style="border-color: var(--pb-border-subtle)">
          <p class="mb-1 text-xs font-medium" style="color: var(--pb-text-muted)">Routing Rules</p>
          <div v-for="rule in ch.routing_rules" :key="rule.id" class="flex items-center justify-between py-0.5">
            <span class="text-xs" style="color: var(--pb-text-secondary)">
              {{ rule.source_filter || 'all sources' }} / {{ rule.severity_filter || 'all severities' }}
            </span>
            <button @click="handleDeleteRule(ch.id, rule.id)" class="text-xs" style="color: var(--pb-status-down)">Remove</button>
          </div>
        </div>

        <!-- Add rule -->
        <div class="mt-2">
          <button
            v-if="showRuleForm !== ch.id"
            @click="showRuleForm = ch.id"
            class="text-xs"
            style="color: var(--pb-accent)"
          >
            + Add routing rule
          </button>
          <div v-else class="mt-1 flex gap-2">
            <input
              v-model="ruleForm.source_filter"
              placeholder="Sources (e.g. endpoint,certificate)"
              class="flex-1 rounded border px-2 py-1 text-xs outline-none"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            />
            <input
              v-model="ruleForm.severity_filter"
              placeholder="Severities (e.g. critical)"
              class="flex-1 rounded border px-2 py-1 text-xs outline-none"
              style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            />
            <button @click="handleAddRule(ch.id)" class="rounded px-2 py-1 text-xs text-white" style="background: var(--pb-accent)">Add</button>
            <button @click="showRuleForm = null" class="text-xs" style="color: var(--pb-text-muted)">Cancel</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
