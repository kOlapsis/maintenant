<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useHeartbeatsStore } from '@/stores/heartbeats'
import { createHeartbeat } from '@/services/heartbeatApi'
import HeartbeatCard from '@/components/HeartbeatCard.vue'
import HeartbeatDetail from '@/components/HeartbeatDetail.vue'

const store = useHeartbeatsStore()
const selectedId = ref<number | null>(null)
const showCreateForm = ref(false)
const createError = ref<string | null>(null)

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
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to create heartbeat'
  }
}
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-white">Heartbeats</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--pb-text-muted)' }">
          Passive cron &amp; scheduled task monitoring
        </p>
      </div>
      <button
        :style="{
          borderRadius: 'var(--pb-radius-lg)',
          backgroundColor: 'var(--pb-accent)',
          color: 'var(--pb-text-inverted)',
          padding: '0.5rem 1rem',
          fontSize: '0.875rem',
          fontWeight: '500',
        }"
        @click="showCreateForm = !showCreateForm"
      >
        {{ showCreateForm ? 'Cancel' : 'New Heartbeat' }}
      </button>
    </div>

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
        {{ createError }}
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

    <!-- Detail view -->
    <HeartbeatDetail
      v-if="selectedId"
      :heartbeat-id="selectedId"
      class="mb-6"
      @close="selectedId = null"
    />

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
        @refresh="store.fetchHeartbeats()"
        @select="selectedId = $event"
      />
    </div>
  </div>
</template>
