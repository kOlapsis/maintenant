<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)

  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. You may not use this file except in compliance
  with one of these licenses.

  AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
  Commercial: See COMMERCIAL-LICENSE.md

  Source: https://github.com/kolapsis/maintenant
-->

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-xl font-semibold" style="color: var(--pb-text-primary)">Webhooks</h1>
      <button
        @click="showCreate = true"
        class="min-h-[44px] rounded px-4 text-sm font-medium"
        style="background-color: var(--pb-accent); color: var(--pb-text-inverted); border-radius: var(--pb-radius-md)"
      >
        + Add Webhook
      </button>
    </div>

    <div v-if="loading" class="text-sm" style="color: var(--pb-text-muted)">Loading...</div>
    <div v-else-if="error" class="text-sm" style="color: var(--pb-status-down)">{{ error }}</div>
    <div
      v-else
      class="overflow-hidden overflow-x-auto"
      style="background-color: var(--pb-bg-surface); border-radius: var(--pb-radius-lg); border: 1px solid var(--pb-border-default)"
    >
      <table class="w-full text-sm min-w-[600px]">
        <thead>
          <tr style="border-bottom: 1px solid var(--pb-border-default)">
            <th class="text-left px-4 py-3 font-medium" style="color: var(--pb-text-muted)">Name</th>
            <th class="text-left px-4 py-3 font-medium hidden sm:table-cell" style="color: var(--pb-text-muted)">URL</th>
            <th class="text-left px-4 py-3 font-medium hidden md:table-cell" style="color: var(--pb-text-muted)">Events</th>
            <th class="text-left px-4 py-3 font-medium" style="color: var(--pb-text-muted)">Status</th>
            <th class="text-left px-4 py-3 font-medium hidden lg:table-cell" style="color: var(--pb-text-muted)">Last Delivery</th>
            <th class="px-4 py-3"></th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="wh in webhooks"
            :key="wh.id"
            style="border-bottom: 1px solid var(--pb-border-subtle)"
          >
            <td class="px-4 py-3" style="color: var(--pb-text-primary)">{{ wh.name }}</td>
            <td class="px-4 py-3 text-xs max-w-48 truncate hidden sm:table-cell" style="color: var(--pb-text-muted)" :title="wh.url">
              {{ wh.url }}
            </td>
            <td class="px-4 py-3 hidden md:table-cell">
              <span
                v-for="et in wh.event_types"
                :key="et"
                class="inline-block text-xs rounded px-2 py-0.5 mr-1 mb-0.5"
                style="background-color: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
              >
                {{ et === '*' ? 'All' : et }}
              </span>
            </td>
            <td class="px-4 py-3">
              <span
                class="text-xs rounded px-2 py-0.5"
                :style="{
                  backgroundColor: wh.is_active ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
                  color: wh.is_active ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
                }"
              >
                {{ wh.is_active ? 'Active' : 'Disabled' }}
              </span>
              <span v-if="wh.failure_count > 0" class="text-xs ml-1" style="color: var(--pb-status-warn)">
                ({{ wh.failure_count }} failures)
              </span>
            </td>
            <td class="px-4 py-3 text-xs hidden lg:table-cell" style="color: var(--pb-text-muted)">
              <template v-if="wh.last_delivery_status">
                <span :style="{ color: wh.last_delivery_status === 'delivered' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)' }">
                  {{ wh.last_delivery_status }}
                </span>
                <br />
                {{ wh.last_delivery_at ? formatDate(wh.last_delivery_at) : '' }}
              </template>
              <span v-else style="color: var(--pb-text-muted)">Never</span>
            </td>
            <td class="px-4 py-3 text-right space-x-2">
              <button
                @click="handleTest(wh)"
                :disabled="testing === wh.id"
                class="text-xs min-h-[36px] px-2 disabled:opacity-50"
                style="color: var(--pb-accent)"
              >
                {{ testing === wh.id ? 'Testing...' : 'Test' }}
              </button>
              <button
                @click="handleDelete(wh)"
                class="text-xs min-h-[36px] px-2"
                style="color: var(--pb-status-down)"
              >
                Delete
              </button>
            </td>
          </tr>
          <tr v-if="!webhooks.length">
            <td colspan="6" class="px-4 py-6 text-center" style="color: var(--pb-text-muted)">No webhooks registered yet</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-if="testResult"
      class="mt-4 p-3 rounded text-sm"
      :style="{
        backgroundColor: testResult.status === 'delivered' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
        color: testResult.status === 'delivered' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-md)',
      }"
    >
      Test result: {{ testResult.status }}
      <span v-if="testResult.http_status"> (HTTP {{ testResult.http_status }})</span>
      <span v-if="testResult.error"> --- {{ testResult.error }}</span>
    </div>

    <WebhookForm v-if="showCreate" @close="showCreate = false" @created="onCreated" />
  </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listWebhooks, deleteWebhook, testWebhook, type WebhookSubscription, type TestWebhookResponse } from '@/services/webhookApi'
import { useConfirm } from '@/composables/useConfirm'
import WebhookForm from '@/components/WebhookForm.vue'

const confirm = useConfirm()

const webhooks = ref<WebhookSubscription[]>([])
const loading = ref(false)
const error = ref('')
const showCreate = ref(false)
const testing = ref<string | null>(null)
const testResult = ref<TestWebhookResponse | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await listWebhooks()
    webhooks.value = res.webhooks
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load webhooks'
  } finally {
    loading.value = false
  }
}

async function handleTest(wh: WebhookSubscription) {
  testing.value = wh.id
  testResult.value = null
  try {
    testResult.value = await testWebhook(wh.id)
  } catch (e: unknown) {
    testResult.value = { status: 'failed', error: e instanceof Error ? e.message : 'Test failed' }
  } finally {
    testing.value = null
  }
}

async function handleDelete(wh: WebhookSubscription) {
  const ok = await confirm({
    title: 'Delete webhook',
    message: `Remove "${wh.name}"? This cannot be undone.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  try {
    await deleteWebhook(wh.id)
    await load()
  } catch {
    // silently ignore
  }
}

function onCreated() {
  showCreate.value = false
  load()
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

onMounted(load)
</script>
