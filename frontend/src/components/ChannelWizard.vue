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
import { ref, computed } from 'vue'
import { createChannel, testChannel } from '@/services/alertApi'
import { useEdition } from '@/composables/useEdition'
import FeatureGate from '@/components/FeatureGate.vue'
import SmtpNotConfigured from '@/components/SmtpNotConfigured.vue'

const { hasFeature, isEnterprise } = useEdition()

const emit = defineEmits<{
  created: [id: number]
  cancel: []
}>()

const step = ref<1 | 2 | 3>(1)
const selectedType = ref<string | null>(null)
const createdChannelId = ref<number | null>(null)
const testStatus = ref<'idle' | 'testing' | 'success' | 'failed'>('idle')
const testError = ref('')

const form = ref({
  name: '',
  url: '',
  headers: '',
  enabled: true,
})

const ceChannelTypes = [
  {
    key: 'discord',
    label: 'Discord',
    description: 'Post alerts to a Discord channel via webhook',
    icon: 'discord',
    urlPlaceholder: 'https://discord.com/api/webhooks/...',
  },
  {
    key: 'webhook',
    label: 'Webhook',
    description: 'HTTP POST to any endpoint with JSON payload',
    icon: 'webhook',
    urlPlaceholder: 'https://api.example.com/hooks/alerts',
  },
]

const proChannelTypes = [
  {
    key: 'email',
    label: 'Email (SMTP)',
    description: 'Send email notifications via your own SMTP server',
    icon: 'email',
    urlPlaceholder: 'alerts@example.com',
    feature: 'smtp',
  },
  {
    key: 'slack',
    label: 'Slack',
    description: 'Send rich notifications to a Slack channel',
    icon: 'slack',
    urlPlaceholder: 'https://hooks.slack.com/services/...',
    feature: 'slack',
  },
  {
    key: 'teams',
    label: 'Teams',
    description: 'Send alerts to Microsoft Teams via webhook',
    icon: 'teams',
    urlPlaceholder: 'https://outlook.office.com/webhook/...',
    feature: 'teams',
  },
]

const allChannelTypes = [...ceChannelTypes, ...proChannelTypes]

const selectedTypeConfig = computed(() =>
  allChannelTypes.find(t => t.key === selectedType.value)
)

function selectType(type: string) {
  selectedType.value = type
  step.value = 2
  form.value.name = ''
  form.value.url = ''
  form.value.headers = ''
}

async function submitConfig() {
  try {
    const result = await createChannel({
      name: form.value.name,
      type: selectedType.value!,
      url: form.value.url,
      headers: form.value.headers || undefined,
      enabled: form.value.enabled,
    })
    createdChannelId.value = result.id
    step.value = 3
  } catch (e) {
    console.error('Failed to create channel:', e)
  }
}

async function runTest() {
  if (!createdChannelId.value) return
  testStatus.value = 'testing'
  testError.value = ''
  try {
    const res = await testChannel(createdChannelId.value)
    if (res.status === 'delivered') {
      testStatus.value = 'success'
    } else {
      testStatus.value = 'failed'
      testError.value = res.error || 'Delivery failed'
    }
  } catch (e) {
    testStatus.value = 'failed'
    testError.value = e instanceof Error ? e.message : 'Test request failed'
  }
}

function finish() {
  if (createdChannelId.value) {
    emit('created', createdChannelId.value)
  }
}

function goBack() {
  if (step.value === 2) {
    step.value = 1
    selectedType.value = null
  } else if (step.value === 3) {
    step.value = 2
  }
}
</script>

<template>
  <div
    class="rounded-lg border p-5"
    style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
  >
    <!-- Step indicator -->
    <div class="mb-5 flex items-center gap-2">
      <template v-for="s in [1, 2, 3]" :key="s">
        <div
          class="flex items-center justify-center w-7 h-7 rounded-full text-xs font-bold transition-all"
          :style="{
            background: step >= s ? 'var(--pb-accent)' : 'var(--pb-bg-elevated)',
            color: step >= s ? '#fff' : 'var(--pb-text-muted)',
          }"
        >
          {{ s }}
        </div>
        <div
          v-if="s < 3"
          class="flex-1 h-0.5 rounded transition-all"
          :style="{
            background: step > s ? 'var(--pb-accent)' : 'var(--pb-border-default)',
          }"
        />
      </template>
    </div>

    <!-- Step 1: Select type -->
    <div v-if="step === 1">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--pb-text-primary)">Select Channel Type</h3>
      <p class="mb-4 text-xs" style="color: var(--pb-text-muted)">Choose how you want to receive notifications</p>

      <!-- CE channels -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <button
          v-for="type in ceChannelTypes"
          :key="type.key"
          @click="selectType(type.key)"
          class="flex flex-col items-center gap-2 rounded-lg border p-4 text-center transition-all"
          :style="{
            background: 'var(--pb-bg-elevated)',
            borderColor: selectedType === type.key ? 'var(--pb-accent)' : 'var(--pb-border-default)',
          }"
          @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--pb-accent)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = selectedType === type.key ? 'var(--pb-accent)' : 'var(--pb-border-default)'"
        >
          <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background: var(--pb-bg-hover)">
            <!-- Discord -->
            <svg v-if="type.icon === 'discord'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: #5865f2">
              <path d="M4 4c2-1.5 4-2 6-2s4 .5 6 2" />
              <path d="M4 16c2 1.5 4 2 6 2s4-.5 6-2" />
              <circle cx="7.5" cy="10" r="1.5" />
              <circle cx="12.5" cy="10" r="1.5" />
            </svg>
            <!-- Webhook -->
            <svg v-else width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--pb-status-warn)">
              <circle cx="10" cy="6" r="3" />
              <path d="M10 9v6" />
              <path d="M6 18l4-3 4 3" />
            </svg>
          </div>
          <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ type.label }}</span>
          <span class="text-[11px]" style="color: var(--pb-text-muted)">{{ type.description }}</span>
        </button>
      </div>

      <!-- Pro channels -->
      <div class="mt-3 grid grid-cols-1 sm:grid-cols-3 gap-3">
        <FeatureGate
          v-for="type in proChannelTypes"
          :key="type.key"
          :feature="type.feature"
          :title="type.label"
          :description="type.description"
        >
          <button
            @click="selectType(type.key)"
            class="flex flex-col items-center gap-2 rounded-lg border p-4 text-center transition-all w-full"
            :style="{
              background: 'var(--pb-bg-elevated)',
              borderColor: selectedType === type.key ? 'var(--pb-accent)' : 'var(--pb-border-default)',
            }"
            @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--pb-accent)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = selectedType === type.key ? 'var(--pb-accent)' : 'var(--pb-border-default)'"
          >
            <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background: var(--pb-bg-hover)">
              <!-- Email -->
              <svg v-if="type.icon === 'email'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--pb-status-ok)">
                <rect x="2" y="4" width="16" height="12" rx="2" />
                <path d="M2 6l8 5 8-5" />
              </svg>
              <!-- Slack -->
              <svg v-else-if="type.icon === 'slack'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--pb-accent)">
                <path d="M6 2v4M14 14v4M2 6h4M14 6h4M6 10h8M10 6v8" />
              </svg>
              <!-- Teams -->
              <svg v-else-if="type.icon === 'teams'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: #6264A7">
                <rect x="3" y="4" width="14" height="12" rx="2" />
                <path d="M7 10h6M10 7v6" />
              </svg>
            </div>
            <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ type.label }}</span>
            <span class="text-[11px]" style="color: var(--pb-text-muted)">{{ type.description }}</span>
          </button>

          <!-- Custom placeholder: Pro active but SMTP not configured -->
          <template v-if="type.feature === 'smtp' && isEnterprise" #placeholder>
            <SmtpNotConfigured :title="type.label" />
          </template>
        </FeatureGate>
      </div>

      <div class="mt-4 flex justify-end">
        <button
          @click="emit('cancel')"
          class="rounded-md border px-3 py-1.5 text-sm"
          style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
        >
          Cancel
        </button>
      </div>
    </div>

    <!-- Step 2: Configure -->
    <div v-else-if="step === 2">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--pb-text-primary)">
        Configure {{ selectedTypeConfig?.label }} Channel
      </h3>
      <p class="mb-4 text-xs" style="color: var(--pb-text-muted)">
        Enter the connection details for your {{ selectedTypeConfig?.label }} integration
      </p>

      <form @submit.prevent="submitConfig" class="space-y-3">
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Channel Name</label>
          <input
            v-model="form.name"
            required
            placeholder="e.g. #ops-alerts"
            class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none"
            style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          />
        </div>
        <div>
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">
            {{ selectedType === 'email' ? 'Email Address' : 'Webhook URL' }}
          </label>
          <input
            v-model="form.url"
            required
            :type="selectedType === 'email' ? 'email' : 'url'"
            :placeholder="selectedTypeConfig?.urlPlaceholder"
            class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none"
            style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          />
        </div>
        <div v-if="selectedType === 'webhook'">
          <label class="block text-xs font-medium" style="color: var(--pb-text-secondary)">Custom Headers (JSON, optional)</label>
          <input
            v-model="form.headers"
            placeholder='{"Authorization": "Bearer ..."}'
            class="mt-1 w-full rounded-md border px-3 py-1.5 text-sm outline-none"
            style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-primary)"
          />
        </div>
        <div class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" id="wizard-enabled" class="rounded" style="accent-color: var(--pb-accent)" />
          <label for="wizard-enabled" class="text-sm" style="color: var(--pb-text-secondary)">Enable channel immediately</label>
        </div>
        <div class="flex justify-between pt-2">
          <button
            type="button"
            @click="goBack"
            class="rounded-md border px-3 py-1.5 text-sm"
            style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
          >
            Back
          </button>
          <button
            type="submit"
            class="rounded-md px-4 py-1.5 text-sm font-medium text-pb-primary"
            style="background: var(--pb-accent)"
          >
            Create & Continue
          </button>
        </div>
      </form>
    </div>

    <!-- Step 3: Test -->
    <div v-else-if="step === 3">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--pb-text-primary)">Test Your Channel</h3>
      <p class="mb-4 text-xs" style="color: var(--pb-text-muted)">
        Send a test notification to verify everything works correctly
      </p>

      <div class="mb-4 rounded-lg border p-4" style="background: var(--pb-bg-elevated); border-color: var(--pb-border-subtle)">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-sm font-medium" style="color: var(--pb-text-primary)">{{ form.name }}</span>
          <span class="rounded px-1.5 py-0.5 text-xs" style="background: var(--pb-bg-hover); color: var(--pb-text-muted)">{{ selectedType }}</span>
        </div>
        <p class="text-xs truncate" style="color: var(--pb-text-muted)">{{ form.url }}</p>
      </div>

      <button
        @click="runTest"
        :disabled="testStatus === 'testing'"
        class="mb-4 w-full rounded-md px-4 py-2 text-sm font-medium text-pb-primary disabled:opacity-50 transition-colors"
        style="background: var(--pb-accent)"
      >
        {{ testStatus === 'testing' ? 'Sending test...' : 'Send Test Notification' }}
      </button>

      <!-- Test result -->
      <div
        v-if="testStatus === 'success'"
        class="mb-4 rounded-lg border p-3 flex items-center gap-2"
        style="background: var(--pb-status-ok-bg); border-color: var(--pb-status-ok)"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" :style="{ color: 'var(--pb-status-ok)' }">
          <path d="M4 8.5L6.5 11L12 5" />
        </svg>
        <span class="text-sm" style="color: var(--pb-status-ok)">Test notification delivered successfully!</span>
      </div>

      <div
        v-if="testStatus === 'failed'"
        class="mb-4 rounded-lg border p-3 flex items-start gap-2"
        style="background: var(--pb-status-down-bg); border-color: var(--pb-status-down)"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="mt-0.5 shrink-0" :style="{ color: 'var(--pb-status-down)' }">
          <line x1="4" y1="4" x2="12" y2="12" /><line x1="12" y1="4" x2="4" y2="12" />
        </svg>
        <div>
          <span class="text-sm font-medium" style="color: var(--pb-status-down)">Test failed</span>
          <p class="text-xs mt-0.5" style="color: var(--pb-status-down)">{{ testError }}</p>
        </div>
      </div>

      <div class="flex justify-between">
        <button
          @click="goBack"
          class="rounded-md border px-3 py-1.5 text-sm"
          style="border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
        >
          Back
        </button>
        <button
          @click="finish"
          class="rounded-md px-4 py-1.5 text-sm font-medium text-pb-primary"
          style="background: var(--pb-accent)"
        >
          Done
        </button>
      </div>
    </div>
  </div>
</template>
