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
import { usePreferencesStore } from '@/stores/preferences'
import { useEdition } from '@/composables/useEdition'
import { useListFilter } from '@/composables/useListFilter'
import { createHeartbeat, type Heartbeat } from '@/services/heartbeatApi'
import HeartbeatCard from '@/components/HeartbeatCard.vue'
import HeartbeatRow from '@/components/HeartbeatRow.vue'
import HeartbeatStatusBadge from '@/components/HeartbeatStatusBadge.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import ListToolbar from '@/components/ui/ListToolbar.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import type { StatusChip } from '@/components/ui/listFilters'
import { formatDeadline, formatInterval, heartbeatTone } from '@/utils/heartbeatFormat'
import { timeAgo } from '@/utils/time'
import { Heart } from 'lucide-vue-next'
import { docUrl } from '@/utils/docs'
import QuotaRefusal from '@/components/QuotaRefusal.vue'
import CountBadge from '@/components/ui/CountBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import CheckboxInput from '@/components/ui/CheckboxInput.vue'

const { getQuota, reload } = useEdition()

const store = useHeartbeatsStore()
const prefs = usePreferencesStore()
const { openDetail } = inject(detailSlideOverKey)!
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

const view = computed(() => prefs.listView('heartbeats'))

const intervalFilter = ref<'' | 'hour' | 'day' | 'beyond'>('')
const alertingOnly = ref(false)

const {
  search,
  status,
  filtered,
  statusCounts,
  activeFilterCount,
  reset: resetSearchAndStatus,
} = useListFilter<Heartbeat>(
  computed(() => store.heartbeats),
  {
    searchFields: (hb) => [hb.name, hb.id],
    status: (hb) => hb.status,
    extra: {
      interval: computed(() => {
        const bucket = intervalFilter.value
        if (!bucket) return null
        return (hb: Heartbeat) => {
          if (bucket === 'hour') return hb.interval_seconds < 3600
          if (bucket === 'day') return hb.interval_seconds >= 3600 && hb.interval_seconds < 86400
          return hb.interval_seconds >= 86400
        }
      }),
      alerting: computed(() =>
        alertingOnly.value ? (hb: Heartbeat) => hb.alert_state === 'alerting' : null,
      ),
    },
  },
)

// Counts come from the list the other filters already narrowed, so a chip
// promising "3 down" really leaves 3 rows standing once it is clicked.
const chips = computed<StatusChip[]>(() =>
  ([
    ['up', 'ok'],
    ['down', 'down'],
    ['started', 'ok'],
    ['new', 'neutral'],
    ['paused', 'warn'],
  ] as const).map(([value, tone]) => ({
    value,
    label: value,
    count: statusCounts.value.get(value) ?? 0,
    tone,
  })),
)

const sortedHeartbeats = computed(() =>
  [...filtered.value].sort((a, b) => a.name.localeCompare(b.name)),
)

const columns: Column[] = [
  { key: 'name', label: 'Name', sortable: true, width: 'minmax(0, 2fr)' },
  { key: 'status', label: 'Status', sortable: true, width: '110px' },
  { key: 'interval', label: 'Interval', sortable: true, align: 'right', width: '90px', priority: 'sm' },
  { key: 'grace', label: 'Grace', align: 'right', width: '80px', priority: 'lg' },
  { key: 'last_ping', label: 'Last ping', sortable: true, align: 'right', width: '110px' },
  { key: 'deadline', label: 'Deadline', align: 'right', width: '120px', priority: 'md' },
]

function sortValue(hb: Heartbeat, key: string): string | number | undefined {
  switch (key) {
    case 'name':
      return hb.name
    case 'status':
      return hb.status
    case 'interval':
      return hb.interval_seconds
    case 'grace':
      return hb.grace_seconds
    case 'last_ping':
      return hb.last_ping_at ? new Date(hb.last_ping_at).getTime() : undefined
    case 'deadline':
      return hb.next_deadline_at ? new Date(hb.next_deadline_at).getTime() : undefined
    default:
      return undefined
  }
}

function resetFilters() {
  resetSearchAndStatus()
  intervalFilter.value = ''
  alertingOnly.value = false
}

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
  <div class="p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Heartbeats</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--mnt-text-muted)' }">
          Passive cron & scheduled task monitoring
        </p>
      </div>
      <div class="flex items-center gap-2">
        <CountBadge
          v-if="!quota.isUnlimited"
          :value="`${quota.used}/${quota.limit}`"
          :tone="quota.isAtLimit ? 'danger' : quota.nearLimit ? 'warn' : 'neutral'"
        />
        <router-link
          v-if="quota.nearLimit && !quota.isAtLimit"
          :to="{ name: 'editions' }"
          class="text-xs font-medium transition-opacity hover:opacity-80"
          style="color: var(--mnt-accent)"
        >
          Upgrade
        </router-link>
        <UiButton
          variant="primary"
          :disabled="quota.isAtLimit"
          :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} heartbeats` : ''"
          @click="showCreateForm = !showCreateForm"
        >
          {{ showCreateForm ? 'Cancel' : 'New Heartbeat' }}
        </UiButton>
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
        <FormField label="Name">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              v-model="form.name"
              placeholder="e.g., Nightly Backup"
              :aria-describedby="describedBy"
              :invalid="invalid"
              required
            />
          </template>
        </FormField>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Expected Interval</label>
          <SegmentedToggle
            :model-value="String(form.interval_seconds)"
            :options="intervalPresets.map((p) => ({ value: String(p.value), label: p.label }))"
            ariaLabel="Expected interval"
            @update:model-value="(v) => (form.interval_seconds = Number(v))"
          />
        </div>
        <FormField label="Grace Period (seconds)">
          <template #default="{ id, describedBy, invalid }">
            <TextInput
              :id="id"
              :model-value="form.grace_seconds"
              type="number"
              min="0"
              :max="form.interval_seconds"
              :aria-describedby="describedBy"
              :invalid="invalid"
              @update:model-value="(v) => (form.grace_seconds = typeof v === 'number' ? v : 0)"
            />
          </template>
        </FormField>
        <UiButton type="submit" variant="primary" class="self-start">
          Create
        </UiButton>
      </form>
    </div>

    <ListToolbar
      scope="heartbeats"
      :search="search"
      :status="status"
      :chips="chips"
      :result-count="filtered.length"
      :active-filter-count="activeFilterCount"
      search-placeholder="Search name or ping token"
      @update:search="search = $event"
      @update:status="status = $event"
      @reset="resetFilters"
    >
      <template #filters>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Expected interval</span>
          <SelectInput
            v-model="intervalFilter"
            size="sm"
            :options="[
              { value: '', label: 'Any interval' },
              { value: 'hour', label: 'Under an hour' },
              { value: 'day', label: 'Under a day' },
              { value: 'beyond', label: 'A day or more' },
            ]"
          />
        </label>

        <div class="flex min-h-[44px] items-center">
          <CheckboxInput v-model="alertingOnly" label="Alerting only" />
        </div>
      </template>
    </ListToolbar>

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
        <UiButton variant="primary" @click="showCreateForm = true">
          Create your first heartbeat
        </UiButton>
      </template>
    </EmptyState>

    <!-- No match for the current search and filters -->
    <EmptyState
      v-else-if="filtered.length === 0"
      :icon="Heart"
      title="No heartbeat matches your filters"
      description="Try a different search term, or clear the filters to see every heartbeat monitor."
    >
      <template #action>
        <UiButton variant="secondary" @click="resetFilters">
          Clear filters
        </UiButton>
      </template>
    </EmptyState>

    <!-- Cards -->
    <div v-else-if="view === 'cards'" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <HeartbeatCard
        v-for="hb in sortedHeartbeats"
        :key="hb.id"
        :heartbeat="hb"
        @refresh="store.fetchHeartbeats(); reload()"
        @select="openDetail('heartbeat', $event)"
      />
    </div>

    <!-- Rows -->
    <div
      v-else-if="view === 'rows'"
      class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface"
    >
      <HeartbeatRow
        v-for="hb in sortedHeartbeats"
        :key="hb.id"
        :heartbeat="hb"
        @select="openDetail('heartbeat', $event)"
      />
    </div>

    <!-- Table -->
    <DataTable
      v-else
      :columns="columns"
      :rows="filtered"
      :row-key="(hb: Heartbeat) => hb.id"
      :sort-value="sortValue"
      :tone="(hb: Heartbeat) => heartbeatTone(hb.status)"
      default-sort="name"
      caption="Heartbeat monitors"
      @select="openDetail('heartbeat', $event.id)"
    >
      <template #cell-name="{ row }">
        <span class="font-medium text-mnt-primary">{{ row.name }}</span>
      </template>
      <template #cell-status="{ row }">
        <HeartbeatStatusBadge :status="row.status" />
      </template>
      <template #cell-interval="{ row }">
        <span class="font-mono tabular-nums">{{ formatInterval(row.interval_seconds) }}</span>
      </template>
      <template #cell-grace="{ row }">
        <span class="font-mono tabular-nums">{{ formatInterval(row.grace_seconds) }}</span>
      </template>
      <template #cell-last_ping="{ row }">
        {{ timeAgo(row.last_ping_at, 'never') }}
      </template>
      <template #cell-deadline="{ row }">
        {{ row.status === 'paused' ? '-' : formatDeadline(row.next_deadline_at) }}
      </template>
    </DataTable>
  </div>
  </div>
</template>
