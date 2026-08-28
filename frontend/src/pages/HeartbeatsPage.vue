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
import { inject, ref, onMounted, onUnmounted } from 'vue'
import { useHeartbeatsStore } from '@/stores/heartbeats'
import { useEdition } from '@/composables/useEdition'
import { createHeartbeat } from '@/services/heartbeatApi'
import HeartbeatCard from '@/components/HeartbeatCard.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { Heart } from 'lucide-vue-next'
import { docUrl } from '@/utils/docs'
import QuotaRefusal from '@/components/QuotaRefusal.vue'

const store = useHeartbeatsStore()
const { openDetail } = inject(detailSlideOverKey)!
const { getQuota, reload } = useEdition()
const quota = getQuota('heartbeats')

const showCreateForm = ref(false)
const createError = ref<unknown>(null)


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
    createError.value = e
  }
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Heartbeats</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--mnt-text-muted)' }">
          Passive cron &amp; scheduled task monitoring
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span
          v-if="!quota.isUnlimited"
          class="rounded-full px-2.5 py-1 text-xs font-medium"
          :style="{
            backgroundColor: quota.isAtLimit ? 'var(--mnt-status-down-bg)' : quota.nearLimit ? 'var(--mnt-status-warn-bg)' : 'var(--mnt-bg-elevated)',
            color: quota.isAtLimit ? 'var(--mnt-status-down)' : quota.nearLimit ? 'var(--mnt-status-warn)' : 'var(--mnt-text-secondary)',
          }"
        >
          {{ quota.used }}/{{ quota.limit }}
        </span>
        <router-link
          v-if="quota.nearLimit && !quota.isAtLimit"
          :to="{ name: 'editions' }"
          class="text-xs font-medium transition-opacity hover:opacity-80"
          style="color: var(--mnt-accent)"
        >
          Upgrade
        </router-link>
        <button
          class="min-h-[44px]"
          :disabled="quota.isAtLimit"
          :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} heartbeats` : ''"
          :style="{
            borderRadius: 'var(--mnt-radius-lg)',
            backgroundColor: 'var(--mnt-accent)',
            color: 'var(--mnt-text-inverted)',
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
      (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">/ping/{uuid}</code>).
      Hit it from a cron job, systemd timer, or any script to report success &mdash; append
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">/$?</code>
      to forward the exit code, or use
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">/start</code>
      + exit code to track duration. If no ping arrives before the deadline (interval + grace), a <em>deadline missed</em> alert fires.
    </FeatureHint>

    <!-- Create form -->
    <div
      v-if="showCreateForm"
      class="mb-6 p-4"
      :style="{
        backgroundColor: 'var(--mnt-bg-surface)',
        border: '1px solid var(--mnt-border-default)',
        borderRadius: 'var(--mnt-radius-lg)',
      }"
    >
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--mnt-text-primary)' }">Create Heartbeat Monitor</h3>
      <QuotaRefusal v-if="createError" :error="createError" />
      <form class="flex flex-col gap-3" @submit.prevent="handleCreate">
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Name</label>
          <input
            v-model="form.name"
            type="text"
            placeholder="e.g., Nightly Backup"
            :style="{
              width: '100%',
              borderRadius: 'var(--mnt-radius-md)',
              border: '1px solid var(--mnt-border-default)',
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Expected Interval</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.interval_seconds === preset.value
                  ? '1px solid var(--mnt-accent)'
                  : '1px solid var(--mnt-border-default)',
                backgroundColor: form.interval_seconds === preset.value
                  ? 'var(--mnt-accent)'
                  : 'transparent',
                color: form.interval_seconds === preset.value
                  ? 'var(--mnt-text-inverted)'
                  : 'var(--mnt-text-secondary)',
              }"
              @click="form.interval_seconds = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Grace Period (seconds)</label>
          <input
            v-model.number="form.grace_seconds"
            type="number"
            min="0"
            :max="form.interval_seconds"
            :style="{
              width: '100%',
              borderRadius: 'var(--mnt-radius-md)',
              border: '1px solid var(--mnt-border-default)',
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-primary)',
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
            borderRadius: 'var(--mnt-radius-lg)',
            backgroundColor: 'var(--mnt-accent)',
            color: 'var(--mnt-text-inverted)',
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
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.up }} up
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.down }} down
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-accent)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.started }} started
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-muted)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.new }} new
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.paused }} paused
      </span>
    </div>

    <!-- Loading -->
    <LoadingSkeleton v-if="store.loading" variant="cards" :count="6" />

    <!-- Error -->
    <ErrorState v-else-if="store.error" :message="store.error" />

    <!-- Empty state -->
    <EmptyState
      v-else-if="store.heartbeats.length === 0"
      :icon="Heart"
      title="No heartbeat monitors"
      description="Heartbeat monitors track cron jobs and scheduled tasks. Create one and integrate the ping URL into your scripts."
    >
      <template #action>
        <button
          class="min-h-[44px] rounded-lg px-4 text-sm font-medium"
          style="background-color: var(--mnt-accent); color: var(--mnt-text-inverted); border-radius: var(--mnt-radius-lg)"
          @click="showCreateForm = true"
        >
          Create your first heartbeat
        </button>
      </template>
    </EmptyState>

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
