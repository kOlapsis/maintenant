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
import SmtpNotConfigured from '@/components/SmtpNotConfigured.vue'
import EditionBadge from '@/components/EditionBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'
import OptionCards from '@/components/ui/OptionCards.vue'

const { hasFeature, editionPermits, requiredEditionFor } = useEdition()

const emit = defineEmits<{
  created: [id: string]
  cancel: []
}>()

const step = ref<1 | 2 | 3>(1)
const selectedType = ref<string | null>(null)
const createdChannelId = ref<string | null>(null)
const testStatus = ref<'idle' | 'testing' | 'success' | 'failed'>('idle')
const testError = ref('')
const submitError = ref('')

const form = ref({
  name: '',
  url: '',
  headers: '',
  // Telegram only: the bot token, and the optional forum topic.
  secret: '',
  threadId: '',
  enabled: true,
})

const openChannelTypes = [
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

const gatedChannelTypes = [
  {
    key: 'email',
    label: 'Email (SMTP)',
    description: 'Send email notifications via your own SMTP server',
    icon: 'email',
    urlPlaceholder: 'alerts@example.com',
    feature: 'smtp',
  },
  {
    key: 'telegram',
    label: 'Telegram',
    description: 'Send alerts to a Telegram chat, group or channel',
    icon: 'telegram',
    urlPlaceholder: '-1001234567890 or @my_public_channel',
    feature: 'telegram',
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

const allChannelTypes = [...openChannelTypes, ...gatedChannelTypes]

const selectedTypeConfig = computed(() =>
  allChannelTypes.find(t => t.key === selectedType.value)
)

const openTypeOptions = computed(() =>
  openChannelTypes.map((t) => ({ value: t.key, label: t.label, description: t.description })),
)

const gatedTypeOptions = computed(() =>
  gatedChannelTypes.map((t) => ({
    value: t.key,
    label: t.label,
    description: t.description,
    disabled: !hasFeature(t.feature),
  })),
)

// What the single destination field means depends on the type: an address, a
// chat id, or a URL. Telegram is the one type where it is not a URL at all.
const destinationLabel = computed(() => {
  if (selectedType.value === 'email') return 'Email Address'
  if (selectedType.value === 'telegram') return 'Chat ID'
  return 'Webhook URL'
})

const destinationInputType = computed(() => {
  if (selectedType.value === 'email') return 'email'
  if (selectedType.value === 'telegram') return 'text'
  return 'url'
})

function selectType(type: string) {
  selectedType.value = type
  step.value = 2
  form.value.name = ''
  form.value.url = ''
  form.value.headers = ''
  form.value.secret = ''
  form.value.threadId = ''
  submitError.value = ''
}

async function submitConfig() {
  submitError.value = ''
  try {
    const isTelegram = selectedType.value === 'telegram'
    const result = await createChannel({
      name: form.value.name,
      type: selectedType.value!,
      url: form.value.url,
      headers: form.value.headers || undefined,
      secret: isTelegram ? form.value.secret : undefined,
      config: isTelegram && form.value.threadId ? { thread_id: form.value.threadId } : undefined,
      enabled: form.value.enabled,
    })
    createdChannelId.value = result.id
    step.value = 3
  } catch (e) {
    submitError.value = e instanceof Error ? e.message : 'Failed to create channel'
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
    style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
  >
    <!-- Step indicator -->
    <div class="mb-5 flex items-center gap-2">
      <template v-for="s in [1, 2, 3]" :key="s">
        <div
          class="flex items-center justify-center w-7 h-7 rounded-full text-xs font-bold transition-all"
          :style="{
            background: step >= s ? 'var(--mnt-accent)' : 'var(--mnt-bg-elevated)',
            color: step >= s ? '#fff' : 'var(--mnt-text-muted)',
          }"
        >
          {{ s }}
        </div>
        <div
          v-if="s < 3"
          class="flex-1 h-0.5 rounded transition-all"
          :style="{
            background: step > s ? 'var(--mnt-accent)' : 'var(--mnt-border-default)',
          }"
        />
      </template>
    </div>

    <!-- Step 1: Select type -->
    <div v-if="step === 1">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--mnt-text-primary)">Select Channel Type</h3>
      <p class="mb-4 text-xs" style="color: var(--mnt-text-muted)">Choose how you want to receive notifications</p>

      <!-- CE channels -->
      <OptionCards
        :model-value="selectedType"
        :options="openTypeOptions"
        ariaLabel="Channel type"
        :columns="2"
        @update:model-value="(v) => v && selectType(v)"
      >
        <template #default="{ option }">
          <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background: var(--mnt-bg-hover)">
            <!-- Discord -->
            <svg v-if="option.value === 'discord'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: #5865f2">
              <path d="M4 4c2-1.5 4-2 6-2s4 .5 6 2" />
              <path d="M4 16c2 1.5 4 2 6 2s4-.5 6-2" />
              <circle cx="7.5" cy="10" r="1.5" />
              <circle cx="12.5" cy="10" r="1.5" />
            </svg>
            <!-- Webhook -->
            <svg v-else width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--mnt-status-warn)">
              <circle cx="10" cy="6" r="3" />
              <path d="M10 9v6" />
              <path d="M6 18l4-3 4 3" />
            </svg>
          </div>
          <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ option.label }}</span>
          <span class="text-[11px]" style="color: var(--mnt-text-muted)">{{ option.description }}</span>
        </template>
      </OptionCards>

      <!-- Channels gated by an edition -->
      <OptionCards
        class="mt-3"
        :model-value="selectedType"
        :options="gatedTypeOptions"
        ariaLabel="Channel type (requires an edition)"
        :columns="3"
        @update:model-value="(v) => v && selectType(v)"
      >
        <template #default="{ option }">
          <!-- SMTP not configured special case -->
          <SmtpNotConfigured
            v-if="option.value === 'email' && editionPermits('smtp') && !hasFeature('smtp')"
            :title="option.label"
          />
          <template v-else>
            <!-- The edition this channel actually needs. It read "Pro" for all
                 three, but email is Personal — a Community user was told to buy
                 the top tier for the middle tier's channel. -->
            <EditionBadge
              v-if="option.disabled && requiredEditionFor(gatedChannelTypes.find((t) => t.key === option.value)!.feature)"
              :edition="requiredEditionFor(gatedChannelTypes.find((t) => t.key === option.value)!.feature)!"
              class="absolute top-2 right-2"
            />

            <div class="w-10 h-10 rounded-lg flex items-center justify-center" style="background: var(--mnt-bg-hover)">
              <!-- Email -->
              <svg v-if="option.value === 'email'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--mnt-status-ok)">
                <rect x="2" y="4" width="16" height="12" rx="2" />
                <path d="M2 6l8 5 8-5" />
              </svg>
              <!-- Telegram -->
              <svg v-else-if="option.value === 'telegram'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: #229ED9">
                <path d="M18 3L2 9.5l4.5 1.7L16 5.5l-7.5 7.2L8 17l2.7-3 3.6 2.7z" />
              </svg>
              <!-- Slack -->
              <svg v-else-if="option.value === 'slack'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: var(--mnt-accent)">
                <path d="M6 2v4M14 14v4M2 6h4M14 6h4M6 10h8M10 6v8" />
              </svg>
              <!-- Teams -->
              <svg v-else-if="option.value === 'teams'" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style="color: #6264A7">
                <rect x="3" y="4" width="14" height="12" rx="2" />
                <path d="M7 10h6M10 7v6" />
              </svg>
            </div>
            <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ option.label }}</span>
            <span class="text-[11px]" style="color: var(--mnt-text-muted)">{{ option.description }}</span>
          </template>
        </template>
      </OptionCards>

      <div class="mt-4 flex justify-end">
        <UiButton variant="secondary" @click="emit('cancel')">Cancel</UiButton>
      </div>
    </div>

    <!-- Step 2: Configure -->
    <div v-else-if="step === 2">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--mnt-text-primary)">
        Configure {{ selectedTypeConfig?.label }} Channel
      </h3>
      <p class="mb-4 text-xs" style="color: var(--mnt-text-muted)">
        Enter the connection details for your {{ selectedTypeConfig?.label }} integration
      </p>

      <form @submit.prevent="submitConfig" class="space-y-3">
        <FormField label="Channel Name" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.name"
              required
              placeholder="e.g. #ops-alerts"
              :aria-describedby="describedBy"
              :invalid="invalid"
            />
          </template>
        </FormField>
        <FormField :label="destinationLabel" required>
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.url"
              required
              :type="destinationInputType"
              :placeholder="selectedTypeConfig?.urlPlaceholder"
              :aria-describedby="describedBy"
              :invalid="invalid"
            />
          </template>
        </FormField>
        <!-- Telegram: a token and an optional topic instead of a URL. The
             destination is fixed by the product, so there is nothing to type. -->
        <template v-if="selectedType === 'telegram'">
          <FormField label="Bot Token" required>
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                v-model="form.secret"
                required
                type="password"
                autocomplete="off"
                placeholder="123456789:AA..."
                :aria-describedby="describedBy"
                :invalid="invalid"
              />
            </template>
          </FormField>
          <FormField label="Topic ID (optional)">
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                v-model="form.threadId"
                placeholder="42"
                :aria-describedby="describedBy"
                :invalid="invalid"
              />
            </template>
          </FormField>
          <div class="text-xs space-y-1" style="color: var(--mnt-text-muted)">
            <p>
              Create a bot with @BotFather to get the token. To find the chat id, send the bot a
              message, then read <code>message.chat.id</code> from
              <code>api.telegram.org/bot&lt;token&gt;/getUpdates</code>.
            </p>
            <ul class="space-y-0.5 pl-4 list-disc">
              <li>Private conversation: a positive number, such as <code>123456789</code>.</li>
              <li>Group or supergroup: a negative number, usually starting with <code>-100</code>. Paste it as it is, minus sign included.</li>
              <li>Public channel: the number, or its <code>@name</code>.</li>
            </ul>
            <p>
              The bot's own <code>@name</code> is not a destination: a bot cannot message itself. A
              private conversation has no <code>@name</code> either, only a number.
            </p>
            <p>Leave the topic empty unless the group uses topics.</p>
          </div>
        </template>
        <FormField v-if="selectedType === 'webhook'" label="Custom Headers (JSON, optional)">
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
        <CheckboxInput v-model="form.enabled" label="Enable channel immediately" />
        <p v-if="submitError" class="text-xs" style="color: var(--mnt-status-down-text)">
          {{ submitError }}
        </p>
        <div class="flex justify-between pt-2">
          <UiButton type="button" variant="secondary" @click="goBack">Back</UiButton>
          <UiButton type="submit" variant="primary">Create & Continue</UiButton>
        </div>
      </form>
    </div>

    <!-- Step 3: Test -->
    <div v-else-if="step === 3">
      <h3 class="mb-1 text-sm font-semibold" style="color: var(--mnt-text-primary)">Test Your Channel</h3>
      <p class="mb-4 text-xs" style="color: var(--mnt-text-muted)">
        Send a test notification to verify everything works correctly
      </p>

      <div class="mb-4 rounded-lg border p-4" style="background: var(--mnt-bg-elevated); border-color: var(--mnt-border-subtle)">
        <div class="flex items-center gap-2 mb-2">
          <span class="text-sm font-medium" style="color: var(--mnt-text-primary)">{{ form.name }}</span>
          <span class="rounded px-1.5 py-0.5 text-xs" style="background: var(--mnt-bg-hover); color: var(--mnt-text-muted)">{{ selectedType }}</span>
        </div>
        <p class="text-xs truncate" style="color: var(--mnt-text-muted)">{{ form.url }}</p>
      </div>

      <UiButton variant="primary" class="mb-4 w-full" :loading="testStatus === 'testing'" @click="runTest">
        {{ testStatus === 'testing' ? 'Sending test...' : 'Send Test Notification' }}
      </UiButton>

      <!-- Test result -->
      <div
        v-if="testStatus === 'success'"
        class="mb-4 rounded-lg border p-3 flex items-center gap-2"
        style="background: var(--mnt-status-ok-bg); border-color: var(--mnt-status-ok)"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" :style="{ color: 'var(--mnt-status-ok)' }">
          <path d="M4 8.5L6.5 11L12 5" />
        </svg>
        <span class="text-sm" style="color: var(--mnt-status-ok)">Test notification delivered successfully!</span>
      </div>

      <div
        v-if="testStatus === 'failed'"
        class="mb-4 rounded-lg border p-3 flex items-start gap-2"
        style="background: var(--mnt-status-down-bg); border-color: var(--mnt-status-down)"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="mt-0.5 shrink-0" :style="{ color: 'var(--mnt-status-down)' }">
          <line x1="4" y1="4" x2="12" y2="12" /><line x1="12" y1="4" x2="4" y2="12" />
        </svg>
        <div>
          <span class="text-sm font-medium" style="color: var(--mnt-status-down)">Test failed</span>
          <p class="text-xs mt-0.5" style="color: var(--mnt-status-down)">{{ testError }}</p>
        </div>
      </div>

      <div class="flex justify-between">
        <UiButton variant="secondary" @click="goBack">Back</UiButton>
        <UiButton variant="primary" @click="finish">Done</UiButton>
      </div>
    </div>
  </div>
</template>
