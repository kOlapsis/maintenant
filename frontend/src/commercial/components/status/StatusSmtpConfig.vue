<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getSmtpConfig, updateSmtpConfig, testSmtp, type SmtpConfig } from '@/services/statusApi'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import UiButton from '@/components/ui/UiButton.vue'

const form = ref<SmtpConfig>({
  host: '',
  port: 587,
  username: '',
  password: '',
  tls_policy: 'opportunistic',
  from_address: '',
  from_name: '',
  configured: false,
  password_set: false,
})

const loading = ref(false)
const saving = ref(false)
const testResult = ref<{ status: string; error?: string } | null>(null)
const saveMessage = ref('')
const passwordTouched = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    const cfg = await getSmtpConfig()
    form.value = { ...cfg, password: '' }
  } catch (e) {
    console.error('Failed to load SMTP config:', e)
  } finally {
    loading.value = false
  }
})

async function handleSave() {
  saving.value = true
  saveMessage.value = ''
  try {
    const payload: Partial<SmtpConfig> = {
      host: form.value.host,
      port: form.value.port,
      username: form.value.username,
      tls_policy: form.value.tls_policy,
      from_address: form.value.from_address,
      from_name: form.value.from_name,
    }
    // Only send password if the user actually typed something new
    if (passwordTouched.value && form.value.password) {
      payload.password = form.value.password
    }
    await updateSmtpConfig(payload)
    saveMessage.value = 'Configuration saved'
    // After save, password is now set if it was provided
    if (passwordTouched.value && form.value.password) {
      form.value.password_set = true
    }
    form.value.password = ''
    passwordTouched.value = false
  } catch (e) {
    saveMessage.value = e instanceof Error ? e.message : 'Failed to save'
  } finally {
    saving.value = false
  }
}

async function handleTest() {
  testResult.value = null
  try {
    testResult.value = await testSmtp()
  } catch (e) {
    testResult.value = { status: 'error', error: e instanceof Error ? e.message : 'Test failed' }
  }
}

function onPasswordInput() {
  passwordTouched.value = true
}
</script>

<template>
  <div>
    <h2 class="mb-4 text-lg font-semibold" style="color: var(--mnt-text-primary)">SMTP Configuration</h2>
    <p class="mb-4 text-sm" style="color: var(--mnt-text-muted)">Configure SMTP to enable email subscriptions for status updates.</p>

    <div v-if="loading" class="text-sm" style="color: var(--mnt-text-muted)">Loading...</div>

    <form v-else @submit.prevent="handleSave" class="max-w-lg space-y-3">
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <FormField label="SMTP Host">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.host" :aria-describedby="describedBy" :invalid="invalid" placeholder="smtp.example.com" />
          </template>
        </FormField>
        <FormField label="Port">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              :model-value="form.port"
              type="number"
              :aria-describedby="describedBy"
              :invalid="invalid"
              @update:model-value="(v) => (form.port = Number(v) || 0)"
            />
          </template>
        </FormField>
      </div>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <FormField label="Username">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.username" :aria-describedby="describedBy" :invalid="invalid" />
          </template>
        </FormField>
        <FormField label="Password">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.password"
              type="password"
              autocomplete="new-password"
              :aria-describedby="describedBy"
              :invalid="invalid"
              :placeholder="form.password_set ? 'Password configured' : ''"
              @input="onPasswordInput"
            />
          </template>
        </FormField>
      </div>
      <FormField label="TLS Policy">
        <template #default="{ id, describedBy, invalid }">
          <SelectInput
            :id="id"
            v-model="form.tls_policy"
            :aria-describedby="describedBy"
            :invalid="invalid"
            :options="[
              { value: 'opportunistic', label: 'Opportunistic' },
              { value: 'mandatory', label: 'Mandatory' },
              { value: 'none', label: 'None' },
            ]"
          />
        </template>
      </FormField>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <FormField label="From Address">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.from_address" type="email" :aria-describedby="describedBy" :invalid="invalid" placeholder="status@example.com" />
          </template>
        </FormField>
        <FormField label="From Name">
          <template #default="{ id, describedBy, invalid }">
            <TextInput :id="id" v-model="form.from_name" :aria-describedby="describedBy" :invalid="invalid" placeholder="maintenant Status" />
          </template>
        </FormField>
      </div>

      <div class="flex items-center gap-3 pt-2">
        <UiButton type="submit" variant="primary" :loading="saving">
          {{ saving ? 'Saving...' : 'Save' }}
        </UiButton>
        <UiButton type="button" variant="secondary" @click="handleTest">Send Test Email</UiButton>
      </div>

      <div v-if="saveMessage" class="rounded border px-3 py-1.5 text-xs" style="background: var(--mnt-status-ok-bg); border-color: var(--mnt-status-ok); color: var(--mnt-status-ok)">
        {{ saveMessage }}
      </div>

      <div
        v-if="testResult"
        class="rounded border px-3 py-1.5 text-xs"
        :style="{
          background: testResult.status === 'sent' ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-down-bg)',
          borderColor: testResult.status === 'sent' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
          color: testResult.status === 'sent' ? 'var(--mnt-status-ok)' : 'var(--mnt-status-down)',
        }"
      >
        {{ testResult.status === 'sent' ? 'Test email sent successfully' : `Failed: ${testResult.error}` }}
      </div>
    </form>
  </div>
</template>
