<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { ref } from 'vue'
import { testSmtp } from '@/services/statusApi'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import UiButton from '@/components/ui/UiButton.vue'
import InlineAlert from '@/components/ui/InlineAlert.vue'

const recipient = ref('')
const sending = ref(false)
const result = ref<{ ok: boolean; message: string } | null>(null)

async function handleTest() {
  const to = recipient.value.trim()
  if (!to) return
  sending.value = true
  result.value = null
  try {
    await testSmtp(to)
    result.value = { ok: true, message: `Test email sent to ${to}` }
  } catch (e) {
    result.value = { ok: false, message: e instanceof Error ? e.message : 'Test failed' }
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="max-w-lg space-y-4">
    <InlineAlert severity="success" tag="SMTP" title="SMTP configured">
      Subscription confirmations and incident notifications are sent through the mail server set by the
      <span class="font-mono">MAINTENANT_SMTP_*</span> environment variables.
    </InlineAlert>

    <form class="space-y-3" @submit.prevent="handleTest">
      <FormField label="Send a test email to">
        <template #default="{ id, describedBy, invalid }">
          <TextInput
            :id="id"
            v-model="recipient"
            type="email"
            autocomplete="email"
            placeholder="you@example.com"
            :aria-describedby="describedBy"
            :invalid="invalid"
          />
        </template>
      </FormField>
      <UiButton type="submit" variant="secondary" :loading="sending" :disabled="!recipient.trim()">
        Send test email
      </UiButton>
      <p
        v-if="result"
        role="status"
        class="text-xs"
        :class="result.ok ? 'text-mnt-status-ok' : 'text-mnt-status-down'"
      >
        {{ result.message }}
      </p>
    </form>
  </div>
</template>
