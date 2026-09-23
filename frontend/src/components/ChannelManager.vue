<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useChannelsStore } from '@/stores/channels'
import { useConfirm } from '@/composables/useConfirm'
import {
  createChannel,
  updateChannel,
  deleteChannel,
  testChannel,
  type NotificationChannel,
} from '@/services/alertApi'
import ChannelWizard from '@/components/ChannelWizard.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'

const store = useChannelsStore()

const showForm = ref(false)
const showWizard = ref(false)
const editingId = ref<string | null>(null)
const form = ref({
  name: '',
  type: 'webhook',
  url: '',
  headers: '',
  // Telegram only. An empty token means "keep the one on file" (FR-006): the
  // server never sends it back, so there is nothing to prefill.
  secret: '',
  threadId: '',
  enabled: true,
})
const editingHasSecret = ref(false)
const testResult = ref<{ id: string; status: string; response_code?: number; error?: string } | null>(null)

function resetForm() {
  form.value = { name: '', type: 'webhook', url: '', headers: '', secret: '', threadId: '', enabled: true }
  editingHasSecret.value = false
  editingId.value = null
  showForm.value = false
}

function startEdit(ch: NotificationChannel) {
  editingId.value = ch.id
  let threadId = ''
  try {
    threadId = ch.config ? (JSON.parse(ch.config).thread_id ?? '') : ''
  } catch {
    threadId = ''
  }
  form.value = {
    name: ch.name,
    type: ch.type,
    url: ch.url,
    headers: ch.headers,
    secret: '',
    threadId,
    enabled: ch.enabled,
  }
  editingHasSecret.value = ch.has_secret ?? false
  showForm.value = true
  showWizard.value = false
}

const isTelegram = computed(() => form.value.type === 'telegram')

const destinationLabel = computed(() => {
  if (form.value.type === 'email') return 'Email Address'
  if (isTelegram.value) return 'Chat ID'
  return 'Webhook URL'
})

const destinationInputType = computed(() => {
  if (form.value.type === 'email') return 'email'
  if (isTelegram.value) return 'text'
  return 'url'
})

/** What a channel shows under its name: a URL for webhooks, the target as-is otherwise. */
function channelTarget(ch: NotificationChannel): string {
  if (ch.type === 'telegram' || ch.type === 'email') return ch.url
  return maskUrl(ch.url)
}

async function submitForm() {
  const payload = {
    name: form.value.name,
    url: form.value.url,
    headers: form.value.headers,
    enabled: form.value.enabled,
    // Omitted when left empty, so the stored token survives an edit (FR-006).
    ...(isTelegram.value && form.value.secret ? { secret: form.value.secret } : {}),
    ...(isTelegram.value ? { config: { thread_id: form.value.threadId } } : {}),
  }
  if (editingId.value) {
    await updateChannel(editingId.value, payload)
  } else {
    await createChannel(payload)
  }
  resetForm()
  store.fetchChannels()
}

const confirm = useConfirm()

async function handleDelete(id: string) {
  const ok = await confirm({
    title: 'Delete channel',
    message:
      'Remove this notification channel? Triggers and escalation policies referencing it will lose this destination.',
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  await deleteChannel(id)
  store.fetchChannels()
}

async function handleTest(id: string) {
  testResult.value = null
  const res = await testChannel(id)
  testResult.value = { id, ...res }
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

function handleWizardCreated() {
  showWizard.value = false
  store.fetchChannels()
}

function editionLabel(edition?: string): string {
  return edition ? edition.charAt(0).toUpperCase() + edition.slice(1) : ''
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold text-mnt-primary">Notification Channels</h2>
      <div class="flex gap-2">
        <UiButton variant="primary" @click="showWizard = true; showForm = false">Add Channel</UiButton>
      </div>
    </div>

    <!-- Pedagogical banner -->
    <div class="mb-4 rounded-xl border border-mnt-default bg-mnt-surface px-4 py-3 text-xs text-mnt-muted">
      Channels are silent by default. To start receiving notifications, wire a channel through an
      <RouterLink to="/alerts/triggers" class="text-mnt-green-400 hover:underline">Alert Trigger</RouterLink>
      or an
      <RouterLink to="/escalation" class="text-mnt-green-400 hover:underline">Escalation Policy</RouterLink>.
    </div>

    <!-- Channel Wizard -->
    <div v-if="showWizard" class="mb-4">
      <ChannelWizard
        @created="handleWizardCreated"
        @cancel="showWizard = false"
      />
    </div>

    <!-- Edit form (for existing channels) -->
    <div v-if="showForm && editingId" class="mb-4 rounded-xl border border-mnt-default bg-mnt-surface p-4">
      <h3 class="mb-3 text-sm font-medium text-mnt-primary">Edit Channel</h3>
      <form @submit.prevent="submitForm" class="space-y-3">
        <FormField label="Name" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.name" required :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField :label="destinationLabel" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.url"
              required
              :type="destinationInputType"
              :aria-describedby="describedBy"
              :invalid="invalid"
            />
          </template>
        </FormField>
        <template v-if="isTelegram">
          <FormField label="Bot Token">
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                v-model="form.secret"
                type="password"
                autocomplete="off"
                :placeholder="editingHasSecret ? 'Token on file — leave empty to keep it' : '123456789:AA...'"
                :aria-describedby="describedBy"
                :invalid="invalid"
              />
            </template>
          </FormField>
          <FormField label="Topic ID (optional)">
            <template #default="{ id, describedBy, invalid }">
              <TextInput :id="id" v-model="form.threadId" placeholder="42" :aria-describedby="describedBy" :invalid="invalid" />
            </template>
          </FormField>
        </template>
        <FormField v-if="!isTelegram" label="Custom Headers (JSON)">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.headers"
              placeholder='{"Authorization": "Bearer ..."}'
              :aria-describedby="describedBy"
              :invalid="invalid"
            />
          </template>
        </FormField>
        <CheckboxInput v-model="form.enabled" label="Enabled" />
        <div class="flex gap-2">
          <UiButton type="submit" variant="primary">Save</UiButton>
          <UiButton type="button" variant="secondary" @click="resetForm">Cancel</UiButton>
        </div>
      </form>
    </div>

    <!-- Channel list -->
    <div class="space-y-3">
      <div
        v-if="store.channels.length === 0 && !store.channelsLoading"
        class="rounded-xl border border-mnt-default bg-mnt-surface p-6 text-center"
      >
        <p class="text-sm text-mnt-muted">No notification channels configured</p>
      </div>

      <div
        v-for="ch in store.channels"
        :key="ch.id"
        class="rounded-xl border border-mnt-default bg-mnt-surface p-4"
      >
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-3">
            <span
              class="h-2 w-2 rounded-full"
              :style="{ background: ch.health === 'healthy' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)' }"
            ></span>
            <div>
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-mnt-primary">{{ ch.name }}</span>
                <span v-if="!ch.enabled" class="rounded px-1.5 py-0.5 text-xs bg-mnt-elevated text-mnt-muted">disabled</span>
                <span
                  v-if="ch.suspended"
                  data-test="channel-suspended"
                  class="suspended-badge rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider"
                  :title="`The running edition no longer delivers through ${ch.type} channels`"
                >Suspended · requires {{ editionLabel(ch.required_edition) }}</span>
                <span class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-mnt-elevated text-mnt-muted">{{ ch.type }}</span>
              </div>
              <p class="text-xs text-mnt-muted">{{ channelTarget(ch) }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <UiButton variant="secondary" size="sm" @click="handleTest(ch.id)">Test</UiButton>
            <UiButton variant="secondary" size="sm" @click="startEdit(ch)">Edit</UiButton>
            <UiButton variant="danger-ghost" size="sm" @click="handleDelete(ch.id)">Delete</UiButton>
          </div>
        </div>

        <!-- Test result -->
        <div
          v-if="testResult && testResult.id === ch.id"
          class="mt-2 rounded border px-3 py-1.5 text-xs"
          :style="{
            background: testResult.status === 'delivered' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-down-bg)',
            borderColor: testResult.status === 'delivered' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
            color: testResult.status === 'delivered' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
          }"
        >
          {{ testResult.status === 'delivered' ? `Delivered (HTTP ${testResult.response_code})` : `Failed: ${testResult.error}` }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.suspended-badge {
  background: var(--mnt-sev-incident-bg);
  border-color: var(--mnt-sev-incident-border);
  color: var(--mnt-sev-incident-text);
}
</style>
