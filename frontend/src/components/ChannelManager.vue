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
import { ref } from 'vue'
import { useChannelsStore } from '@/stores/channels'
import { useConfirm } from '@/composables/useConfirm'
import { createChannel, updateChannel, deleteChannel, testChannel } from '@/services/alertApi'
import ChannelWizard from '@/components/ChannelWizard.vue'

const store = useChannelsStore()

const showForm = ref(false)
const showWizard = ref(false)
const editingId = ref<string | null>(null)
const form = ref({ name: '', url: '', headers: '', enabled: true })
const testResult = ref<{ id: string; status: string; response_code?: number; error?: string } | null>(null)

function resetForm() {
  form.value = { name: '', url: '', headers: '', enabled: true }
  editingId.value = null
  showForm.value = false
}

function startEdit(ch: { id: string; name: string; url: string; headers: string; enabled: boolean }) {
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

function handleWizardCreated(_id: string) {
  showWizard.value = false
  store.fetchChannels()
}
</script>

<template>
  <div>
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-lg font-semibold text-mnt-primary">Notification Channels</h2>
      <div class="flex gap-2">
        <button
          @click="showWizard = true; showForm = false"
          class="inline-flex items-center gap-2 px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted rounded-lg text-xs font-bold transition-all shadow-lg shadow-mnt-green-500/20"
        >
          Add Channel
        </button>
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
        <div>
          <label class="block text-[10px] font-bold uppercase tracking-widest text-mnt-muted">Name</label>
          <input v-model="form.name" required class="mt-1 w-full rounded-lg border border-mnt-default bg-mnt-primary px-3 py-2 text-sm text-mnt-primary focus:outline-none focus:border-mnt-default" />
        </div>
        <div>
          <label class="block text-[10px] font-bold uppercase tracking-widest text-mnt-muted">Webhook URL</label>
          <input v-model="form.url" required type="url" class="mt-1 w-full rounded-lg border border-mnt-default bg-mnt-primary px-3 py-2 text-sm text-mnt-primary focus:outline-none focus:border-mnt-default" />
        </div>
        <div>
          <label class="block text-[10px] font-bold uppercase tracking-widest text-mnt-muted">Custom Headers (JSON)</label>
          <input v-model="form.headers" placeholder='{"Authorization": "Bearer ..."}' class="mt-1 w-full rounded-lg border border-mnt-default bg-mnt-primary px-3 py-2 text-sm text-mnt-primary placeholder:text-mnt-muted focus:outline-none focus:border-mnt-default" />
        </div>
        <div class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" id="ch-enabled" class="rounded accent-mnt-green-500" />
          <label for="ch-enabled" class="text-sm text-mnt-secondary">Enabled</label>
        </div>
        <div class="flex gap-2">
          <button type="submit" class="px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted rounded-lg text-xs font-bold transition-all">Save</button>
          <button type="button" @click="resetForm" class="px-4 py-2 rounded-lg border border-mnt-default text-xs text-mnt-secondary hover:bg-mnt-elevated transition-all">Cancel</button>
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
                <span class="rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wider bg-mnt-elevated text-mnt-muted">{{ ch.type }}</span>
              </div>
              <p class="text-xs text-mnt-muted">{{ maskUrl(ch.url) }}</p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button @click="handleTest(ch.id)" class="rounded-lg border border-mnt-default px-2.5 py-1 text-xs text-mnt-secondary hover:bg-mnt-elevated transition-all">Test</button>
            <button @click="startEdit(ch)" class="rounded-lg border border-mnt-default px-2.5 py-1 text-xs text-mnt-secondary hover:bg-mnt-elevated transition-all">Edit</button>
            <button @click="handleDelete(ch.id)" class="rounded-lg border border-mnt-status-down/40 px-2.5 py-1 text-xs text-mnt-status-down hover:bg-mnt-status-down/10 transition-all">Delete</button>
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
