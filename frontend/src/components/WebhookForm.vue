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
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[10001] flex items-center justify-center"
      @keydown.esc="emit('close')"
    >
      <div
        class="fixed inset-0 bg-black/70 backdrop-blur-sm"
        @click="emit('close')"
      />

      <div
        class="relative mx-4 w-full max-w-md overflow-hidden"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-lg)',
          boxShadow: 'var(--pb-shadow-elevated)',
        }"
        role="dialog"
        aria-modal="true"
        aria-labelledby="webhook-form-title"
      >
        <div class="p-6">
          <h2
            id="webhook-form-title"
            class="text-lg font-semibold mb-4"
            :style="{ color: 'var(--pb-text-primary)' }"
          >
            Add Webhook
          </h2>

          <form @submit.prevent="submit" class="space-y-4">
            <div>
              <label
                class="block text-sm mb-1"
                :style="{ color: 'var(--pb-text-muted)' }"
              >
                Name
              </label>
              <input
                v-model="name"
                type="text"
                maxlength="100"
                required
                placeholder="e.g., Slack Integration"
                class="w-full rounded px-3 py-2 text-sm focus:outline-none"
                :style="{
                  backgroundColor: 'var(--pb-bg-elevated)',
                  border: '1px solid var(--pb-border-default)',
                  color: 'var(--pb-text-primary)',
                  borderRadius: 'var(--pb-radius-md)',
                }"
              />
            </div>

            <div>
              <label
                class="block text-sm mb-1"
                :style="{ color: 'var(--pb-text-muted)' }"
              >
                URL (HTTPS)
              </label>
              <input
                v-model="url"
                type="url"
                required
                placeholder="https://hooks.example.com/webhook"
                class="w-full rounded px-3 py-2 text-sm focus:outline-none"
                :style="{
                  backgroundColor: 'var(--pb-bg-elevated)',
                  border: '1px solid var(--pb-border-default)',
                  color: 'var(--pb-text-primary)',
                  borderRadius: 'var(--pb-radius-md)',
                }"
              />
            </div>

            <div>
              <label
                class="block text-sm mb-1"
                :style="{ color: 'var(--pb-text-muted)' }"
              >
                Secret (optional, for HMAC signing)
              </label>
              <input
                v-model="secret"
                type="text"
                placeholder="Optional signing secret"
                class="w-full rounded px-3 py-2 text-sm focus:outline-none"
                :style="{
                  backgroundColor: 'var(--pb-bg-elevated)',
                  border: '1px solid var(--pb-border-default)',
                  color: 'var(--pb-text-primary)',
                  borderRadius: 'var(--pb-radius-md)',
                }"
              />
            </div>

            <div>
              <label
                class="block text-sm mb-2"
                :style="{ color: 'var(--pb-text-muted)' }"
              >
                Event Types
              </label>
              <div class="space-y-2">
                <label class="flex items-center gap-2">
                  <input
                    type="checkbox"
                    value="*"
                    v-model="selectedEvents"
                    @change="onAllEventsToggle"
                    class="rounded accent-pb-green-500"
                  />
                  <span
                    class="text-sm"
                    :style="{ color: 'var(--pb-text-secondary)' }"
                  >
                    All events
                  </span>
                </label>
                <label
                  v-for="et in specificEventTypes"
                  :key="et.value"
                  class="flex items-center gap-2 ml-4"
                >
                  <input
                    type="checkbox"
                    :value="et.value"
                    v-model="selectedEvents"
                    :disabled="selectedEvents.includes('*')"
                    class="rounded accent-pb-green-500"
                  />
                  <span
                    class="text-sm"
                    :style="{ color: 'var(--pb-text-secondary)' }"
                  >
                    {{ et.label }}
                  </span>
                </label>
              </div>
            </div>

            <div
              v-if="error"
              class="text-sm"
              :style="{ color: 'var(--pb-status-down-text)' }"
            >
              {{ error }}
            </div>

            <div class="flex gap-2 justify-end">
              <button
                type="button"
                @click="emit('close')"
                class="cursor-pointer rounded-lg px-4 py-2 text-sm font-medium transition-colors min-h-[36px]"
                :style="{
                  color: 'var(--pb-text-secondary)',
                  backgroundColor: 'transparent',
                  border: '1px solid var(--pb-border-default)',
                }"
              >
                Cancel
              </button>
              <button
                type="submit"
                :disabled="submitting || !name || !url || selectedEvents.length === 0"
                class="cursor-pointer rounded-lg px-4 py-2 text-sm font-medium transition-colors min-h-[36px] disabled:opacity-50 disabled:cursor-not-allowed"
                :style="{
                  backgroundColor: 'var(--pb-accent)',
                  color: 'var(--pb-text-inverted)',
                }"
              >
                {{ submitting ? 'Creating...' : 'Add Webhook' }}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { createWebhook } from '@/services/webhookApi'

const emit = defineEmits<{
  close: []
  created: []
}>()

const specificEventTypes = [
  { value: 'container.state_changed', label: 'Container state changed' },
  { value: 'endpoint.status_changed', label: 'Endpoint status changed' },
  { value: 'heartbeat.status_changed', label: 'Heartbeat status changed' },
  { value: 'certificate.status_changed', label: 'Certificate status changed' },
  { value: 'alert.fired', label: 'Alert fired' },
  { value: 'alert.resolved', label: 'Alert resolved' },
]

const name = ref('')
const url = ref('')
const secret = ref('')
const selectedEvents = ref<string[]>(['*'])
const submitting = ref(false)
const error = ref('')

function onAllEventsToggle() {
  if (selectedEvents.value.includes('*')) {
    selectedEvents.value = ['*']
  }
}

async function submit() {
  submitting.value = true
  error.value = ''
  try {
    await createWebhook({
      name: name.value,
      url: url.value,
      secret: secret.value || undefined,
      event_types: selectedEvents.value,
    })
    emit('created')
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to create webhook'
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  document.body.style.overflow = 'hidden'
})

onBeforeUnmount(() => {
  document.body.style.overflow = ''
})
</script>
