<template>
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50" @click.self="$emit('close')">
    <div class="bg-gray-800 rounded-lg p-6 w-full max-w-md">
      <h2 class="text-lg font-semibold text-white mb-4">Add Webhook</h2>

      <form @submit.prevent="submit" class="space-y-4">
        <div>
          <label class="block text-sm text-gray-400 mb-1">Name</label>
          <input
            v-model="name"
            type="text"
            maxlength="100"
            required
            placeholder="e.g., Slack Integration"
            class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
          />
        </div>

        <div>
          <label class="block text-sm text-gray-400 mb-1">URL (HTTPS)</label>
          <input
            v-model="url"
            type="url"
            required
            placeholder="https://hooks.example.com/webhook"
            class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
          />
        </div>

        <div>
          <label class="block text-sm text-gray-400 mb-1">Secret (optional, for HMAC signing)</label>
          <input
            v-model="secret"
            type="text"
            placeholder="Optional signing secret"
            class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-blue-500 focus:outline-none"
          />
        </div>

        <div>
          <label class="block text-sm text-gray-400 mb-2">Event Types</label>
          <div class="space-y-2">
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                value="*"
                v-model="selectedEvents"
                @change="onAllEventsToggle"
                class="rounded border-gray-600 bg-gray-900 text-blue-600"
              />
              <span class="text-sm text-gray-300">All events</span>
            </label>
            <label v-for="et in specificEventTypes" :key="et.value" class="flex items-center gap-2 ml-4">
              <input
                type="checkbox"
                :value="et.value"
                v-model="selectedEvents"
                :disabled="selectedEvents.includes('*')"
                class="rounded border-gray-600 bg-gray-900 text-blue-600"
              />
              <span class="text-sm text-gray-300">{{ et.label }}</span>
            </label>
          </div>
        </div>

        <div v-if="error" class="text-red-400 text-sm">{{ error }}</div>

        <div class="flex gap-2 justify-end">
          <button
            type="button"
            @click="$emit('close')"
            class="text-sm text-gray-400 hover:text-gray-300 px-4 py-2"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="submitting || !name || !url || selectedEvents.length === 0"
            class="bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white rounded px-4 py-2 text-sm font-medium"
          >
            {{ submitting ? 'Creating...' : 'Add Webhook' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
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
</script>
