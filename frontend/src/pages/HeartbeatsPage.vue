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
import { inject, ref, computed, onMounted, onUnmounted } from 'vue'
import { useHeartbeatsStore } from '@/stores/heartbeats'
import { useEdition } from '@/composables/useEdition'
import { createHeartbeat } from '@/services/heartbeatApi'
import HeartbeatCard from '@/components/HeartbeatCard.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'

const store = useHeartbeatsStore()
const { openDetail } = inject(detailSlideOverKey)!
const { getQuota, reload } = useEdition()
const quota = getQuota('heartbeats')

const showCreateForm = ref(false)
const createError = ref<string | null>(null)

const isQuotaError = computed(() => {
  return createError.value?.includes('Upgrade to Pro') || false
})

const form = ref({
  name: '',
  interval_seconds: 300,
  grace_seconds: 60,
})

const intervalPresets = [
  { label: '1m', value: 60 },
  { label: '5m', value: 300 },
  { label: '15m', value: 900 },
  { label: '1h', value: 3600 },
  { label: '6h', value: 21600 },
  { label: '12h', value: 43200 },
  { label: '24h', value: 86400 },
  { label: '7d', value: 604800 },
]

onMounted(() => {
  store.fetchHeartbeats()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})

async function handleCreate() {
  createError.value = null
  try {
    await createHeartbeat(form.value)
    showCreateForm.value = false
    form.value = { name: '', interval_seconds: 300, grace_seconds: 60 }
    store.fetchHeartbeats()
    reload()
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to create heartbeat'
  }
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-pb-primary">Heartbeats</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--pb-text-muted)' }">
          Passive cron &amp; scheduled task monitoring
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span
          v-if="!quota.isUnlimited"
          class="rounded-full px-2.5 py-1 text-xs font-medium"
          :style="{
            backgroundColor: quota.isAtLimit ? 'var(--pb-status-down-bg)' : quota.nearLimit ? 'var(--pb-status-warn-bg)' : 'var(--pb-bg-elevated)',
            color: quota.isAtLimit ? 'var(--pb-status-down)' : quota.nearLimit ? 'var(--pb-status-warn)' : 'var(--pb-text-secondary)',
          }"
        >
          {{ quota.used }}/{{ quota.limit }}
        </span>
        <router-link
          v-if="quota.nearLimit && !quota.isAtLimit"
          :to="{ name: 'pro-edition' }"
          class="text-xs font-medium transition-opacity hover:opacity-80"
          style="color: var(--pb-accent)"
        >
          Upgrade
        </router-link>
        <button
          class="min-h-[44px]"
          :disabled="quota.isAtLimit"
          :title="quota.isAtLimit ? `Community edition limited to ${quota.limit} heartbeats` : ''"
          :style="{
            borderRadius: 'var(--pb-radius-lg)',
            backgroundColor: 'var(--pb-accent)',
            color: 'var(--pb-text-inverted)',
            padding: '0.5rem 1rem',
            fontSize: '0.875rem',
            fontWeight: '500',
            opacity: quota.isAtLimit ? '0.5' : '1',
            cursor: quota.isAtLimit ? 'not-allowed' : 'pointer',
          }"
          @click="showCreateForm = !showCreateForm"
        >
          {{ showCreateForm ? 'Cancel' : 'New Heartbeat' }}
        </button>
      </div>
    </div>

    <FeatureHint
      storage-key="heartbeats"
      title="Monitor cron jobs with a single curl"
      :doc-href="docUrl('features/heartbeats/#ping-url-format')"
    >
      Each monitor gets a unique public ping URL
      (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">/ping/{uuid}</code>).
      Hit it from a cron job, systemd timer, or any script to report success &mdash; append
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">/$?</code>
      to forward the exit code, or use
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">/start</code>
      + exit code to track duration. If no ping arrives before the deadline (interval + grace), a <em>deadline missed</em> alert fires.
    </FeatureHint>

    <!-- Create form -->
    <div
      v-if="showCreateForm"
      class="mb-6 p-4"
      :style="{
        backgroundColor: 'var(--pb-bg-surface)',
        border: '1px solid var(--pb-border-default)',
        borderRadius: 'var(--pb-radius-lg)',
      }"
    >
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">Create Heartbeat Monitor</h3>
      <div
        v-if="createError"
        class="mb-3 rounded p-2 text-sm"
        :style="{
          backgroundColor: 'var(--pb-status-down-bg)',
          color: 'var(--pb-status-down)',
          borderRadius: 'var(--pb-radius-sm)',
        }"
      >
        <template v-if="isQuotaError">
          {{ createError.split('Upgrade to Pro')[0] }}
          <a
            href="/pro-edition"
            class="font-medium underline transition-opacity hover:opacity-80"
            style="color: #a78bfa"
          >
            Upgrade to Pro
          </a>
          {{ createError.split('Upgrade to Pro')[1] }}
        </template>
        <template v-else>
          {{ createError }}
        </template>
      </div>
      <form class="flex flex-col gap-3" @submit.prevent="handleCreate">
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Name</label>
          <input
            v-model="form.name"
            type="text"
            placeholder="e.g., Nightly Backup"
            :style="{
              width: '100%',
              borderRadius: 'var(--pb-radius-md)',
              border: '1px solid var(--pb-border-default)',
              backgroundColor: 'var(--pb-bg-elevated)',
              color: 'var(--pb-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Expected Interval</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.interval_seconds === preset.value
                  ? '1px solid var(--pb-accent)'
                  : '1px solid var(--pb-border-default)',
                backgroundColor: form.interval_seconds === preset.value
                  ? 'var(--pb-accent)'
                  : 'transparent',
                color: form.interval_seconds === preset.value
                  ? 'var(--pb-text-inverted)'
                  : 'var(--pb-text-secondary)',
              }"
              @click="form.interval_seconds = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Grace Period (seconds)</label>
          <input
            v-model.number="form.grace_seconds"
            type="number"
            min="0"
            :max="form.interval_seconds"
            :style="{
              width: '100%',
              borderRadius: 'var(--pb-radius-md)',
              border: '1px solid var(--pb-border-default)',
              backgroundColor: 'var(--pb-bg-elevated)',
              color: 'var(--pb-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              minHeight: '44px',
            }"
          />
        </div>
        <button
          type="submit"
          :style="{
            alignSelf: 'flex-start',
            borderRadius: 'var(--pb-radius-lg)',
            backgroundColor: 'var(--pb-accent)',
            color: 'var(--pb-text-inverted)',
            padding: '0.5rem 1rem',
            fontSize: '0.875rem',
            fontWeight: '500',
          }"
        >
          Create
        </button>
      </form>
    </div>

    <!-- Status summary -->
    <div class="mb-6 flex gap-4 text-sm">
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-ok-bg)', color: 'var(--pb-status-ok)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.up }} up
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.down }} down
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-ok-bg)', color: 'var(--pb-accent)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.started }} started
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-muted)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.new }} new
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.paused }} paused
      </span>
    </div>

    <!-- Loading -->
    <div v-if="store.loading" class="py-12 text-center" :style="{ color: 'var(--pb-text-muted)' }">
      Loading heartbeats...
    </div>

    <!-- Error -->
    <div
      v-else-if="store.error"
      class="rounded-lg p-4 text-sm"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        border: '1px solid var(--pb-status-down)',
        color: 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-lg)',
      }"
    >
      {{ store.error }}
    </div>

    <!-- Empty state -->
    <div
      v-else-if="store.heartbeats.length === 0"
      class="flex flex-col items-center justify-center py-16 text-center"
    >
      <svg width="56" height="56" viewBox="0 0 56 56" fill="none" stroke="currentColor" stroke-width="1.5" class="mb-4" style="color: var(--pb-text-muted)">
        <rect x="8" y="8" width="40" height="40" rx="8" />
        <path d="M18 28l4 4 6-8 4 4 6-8" stroke-linecap="round" stroke-linejoin="round" />
        <circle cx="28" cy="38" r="2" fill="currentColor" stroke="none" />
      </svg>
      <h3 class="text-lg font-medium mb-1" style="color: var(--pb-text-primary)">No heartbeat monitors</h3>
      <p class="text-sm mb-4 max-w-sm" style="color: var(--pb-text-muted)">
        Heartbeat monitors track cron jobs and scheduled tasks. Create one and integrate the ping URL into your scripts.
      </p>
      <button
        class="min-h-[44px] rounded-lg px-4 text-sm font-medium"
        style="background-color: var(--pb-accent); color: var(--pb-text-inverted); border-radius: var(--pb-radius-lg)"
        @click="showCreateForm = true"
      >
        Create Your First Heartbeat
      </button>
    </div>

    <!-- Heartbeat grid -->
    <div
      v-else
      class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
    >
      <HeartbeatCard
        v-for="hb in store.heartbeats"
        :key="hb.id"
        :heartbeat="hb"
        @refresh="store.fetchHeartbeats(); reload()"
        @select="openDetail('heartbeat', $event)"
      />
    </div>
  </div>
  </div>
</template>
