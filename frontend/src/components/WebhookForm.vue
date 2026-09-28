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
  <UiModal v-model:open="isOpen" title="Add Webhook" size="sm">
    <form id="webhook-form" @submit.prevent="submit" class="space-y-4">
      <FormField label="Name" required>
        <template #default="{ id, describedBy, invalid }">
          <TextInput
            :id="id"
            v-model="name"
            maxlength="100"
            required
            placeholder="e.g., Slack Integration"
            :aria-describedby="describedBy"
            :invalid="invalid"
          />
        </template>
      </FormField>

      <FormField label="URL (HTTPS)" required>
        <template #default="{ id, describedBy, invalid }">
          <TextInput
            :id="id"
            v-model="url"
            type="url"
            required
            placeholder="https://hooks.example.com/webhook"
            :aria-describedby="describedBy"
            :invalid="invalid"
          />
        </template>
      </FormField>

      <FormField label="Secret (optional, for HMAC signing)">
        <template #default="{ id, describedBy, invalid }">
          <TextInput
            :id="id"
            v-model="secret"
            placeholder="Optional signing secret"
            :aria-describedby="describedBy"
            :invalid="invalid"
          />
        </template>
      </FormField>

      <div>
        <label class="mb-2 block text-sm text-mnt-muted">Event Types</label>
        <div class="space-y-2">
          <CheckboxInput v-model="selectedEvents" value="*" label="All events" @change="onAllEventsToggle" />
          <div v-for="et in specificEventTypes" :key="et.value" class="ml-4">
            <CheckboxInput
              v-model="selectedEvents"
              :value="et.value"
              :label="et.label"
              :disabled="selectedEvents.includes('*')"
            />
          </div>
        </div>
      </div>

      <div v-if="error" class="text-sm text-mnt-status-down-text">
        {{ error }}
      </div>
    </form>

    <template #footer>
      <UiButton variant="secondary" @click="emit('close')">Cancel</UiButton>
      <UiButton
        variant="primary"
        type="submit"
        form="webhook-form"
        :loading="submitting"
        :disabled="!name || !url || selectedEvents.length === 0"
      >
        {{ submitting ? 'Creating...' : 'Add Webhook' }}
      </UiButton>
    </template>
  </UiModal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { createWebhook } from '@/services/webhookApi'
import UiModal from '@/components/ui/UiModal.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

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

const isOpen = ref(true)
watch(isOpen, (val) => {
  if (!val) emit('close')
})

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
