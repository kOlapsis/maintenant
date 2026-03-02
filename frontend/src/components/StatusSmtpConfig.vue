<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getSmtpConfig, updateSmtpConfig, testSmtp, type SmtpConfig } from '@/services/statusApi'

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
    <h2 class="mb-4 text-lg font-semibold" style="color: var(--pb-text-primary)">SMTP Configuration</h2>
    <p class="mb-4 text-sm" style="color: var(--pb-text-muted)">Configure SMTP to enable email subscriptions for status updates.</p>

    <div v-if="loading" class="text-sm" style="color: var(--pb-text-muted)">Loading...</div>

    <form v-else @submit.prevent="handleSave" class="max-w-lg space-y-3">
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">SMTP Host</label>
          <input v-model="form.host" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" placeholder="smtp.example.com" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Port</label>
          <input v-model.number="form.port" type="number" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Username</label>
          <input v-model="form.username" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Password</label>
          <input
            v-model="form.password"
            @input="onPasswordInput"
            type="password"
            class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none"
            style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
            :placeholder="form.password_set ? 'Password configured' : ''"
          />
        </div>
      </div>
      <div>
        <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">TLS Policy</label>
        <select v-model="form.tls_policy" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)">
          <option value="opportunistic">Opportunistic</option>
          <option value="mandatory">Mandatory</option>
          <option value="none">None</option>
        </select>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">From Address</label>
          <input v-model="form.from_address" type="email" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" placeholder="status@example.com" />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">From Name</label>
          <input v-model="form.from_name" class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)" placeholder="maintenant Status" />
        </div>
      </div>

      <div class="flex items-center gap-3 pt-2">
        <button
          type="submit"
          :disabled="saving"
          class="rounded-md px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
          style="background: var(--pb-accent)"
        >
          {{ saving ? 'Saving...' : 'Save' }}
        </button>
        <button
          type="button"
          @click="handleTest"
          class="rounded-md border px-4 py-1.5 text-sm"
          style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
        >
          Send Test Email
        </button>
      </div>

      <div v-if="saveMessage" class="rounded border px-3 py-1.5 text-xs" style="background: var(--pb-status-ok-bg); border-color: var(--pb-status-ok); color: var(--pb-status-ok)">
        {{ saveMessage }}
      </div>

      <div
        v-if="testResult"
        class="rounded border px-3 py-1.5 text-xs"
        :style="{
          background: testResult.status === 'sent' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-down-bg)',
          borderColor: testResult.status === 'sent' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
          color: testResult.status === 'sent' ? 'var(--pb-status-ok)' : 'var(--pb-status-down)',
        }"
      >
        {{ testResult.status === 'sent' ? 'Test email sent successfully' : `Failed: ${testResult.error}` }}
      </div>
    </form>
  </div>
</template>
