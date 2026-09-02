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
import { ref, onMounted, onUnmounted, computed, inject } from 'vue'
import { useEndpointsStore } from '@/stores/endpoints'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { createEndpoint, type Endpoint } from '@/services/endpointApi'
import EndpointCard from '@/components/EndpointCard.vue'
import EndpointRow from '@/components/EndpointRow.vue'
import { Globe } from 'lucide-vue-next'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import ListToolbar from '@/components/ui/ListToolbar.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import EndpointStatusBadge from '@/components/EndpointStatusBadge.vue'
import type { StatusChip } from '@/components/ui/listFilters'
import { usePreferencesStore } from '@/stores/preferences'
import { endpointTone } from '@/utils/endpointTone'
import { timeAgo } from '@/utils/time'
import { docUrl } from '@/utils/docs'
import QuotaRefusal from '@/components/QuotaRefusal.vue'

const store = useEndpointsStore()
const containers = useContainersStore()
const prefs = usePreferencesStore()
const { getQuota, reload } = useEdition()
const quota = getQuota('endpoints')
const { openDetail } = inject(detailSlideOverKey)!

const view = computed(() => prefs.listView('endpoints'))

const statusChips = computed<StatusChip[]>(() => [
  { value: 'up', label: 'up', count: store.statusCounts.up, tone: 'ok' },
  { value: 'down', label: 'down', count: store.statusCounts.down, tone: 'down' },
  { value: 'degraded', label: 'degraded', count: store.statusCounts.degraded, tone: 'warn' },
  { value: 'unknown', label: 'unknown', count: store.statusCounts.unknown, tone: 'unknown' },
])

const hasFilters = computed(
  () => !!store.searchQuery || !!store.statusFilter || store.activeFilterCount > 0,
)

function onRetiredChange() {
  store.fetchEndpoints()
}

const columns: Column[] = [
  { key: 'target', label: 'Target', sortable: true },
  { key: 'type', label: 'Type', width: '68px', priority: 'sm' },
  { key: 'source', label: 'Source', width: 'minmax(0, 0.6fr)', priority: 'md' },
  { key: 'status', label: 'Status', sortable: true, width: '110px' },
  { key: 'response', label: 'Response', sortable: true, align: 'right', width: '90px', priority: 'sm' },
  { key: 'interval', label: 'Interval', width: '80px', priority: 'lg' },
  { key: 'last_check', label: 'Last check', sortable: true, align: 'right', width: '110px', priority: 'md' },
]

function sourceOf(ep: Endpoint): string {
  return ep.source === 'standalone' ? ep.name || 'standalone' : ep.container_name
}

function formatResponseTime(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`
}

function sortValue(ep: Endpoint, key: string): string | number | undefined {
  switch (key) {
    case 'target':
      return ep.target
    case 'status':
      return ep.status
    case 'response':
      return ep.last_response_time_ms
    case 'last_check':
      return ep.last_check_at ? new Date(ep.last_check_at).getTime() : undefined
    default:
      return undefined
  }
}

const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')

const showCreateForm = ref(false)
const createError = ref<unknown>(null)
const creating = ref(false)


const form = ref({
  name: '',
  target: '',
  endpoint_type: 'http' as 'http' | 'tcp',
  interval: '30s',
})

const intervalPresets = [
  { label: '10s', value: '10s' },
  { label: '30s', value: '30s' },
  { label: '1m', value: '1m0s' },
  { label: '5m', value: '5m0s' },
  { label: '15m', value: '15m0s' },
]

function resetForm() {
  form.value = { name: '', target: '', endpoint_type: 'http', interval: '30s' }
  createError.value = null
}

async function handleCreate() {
  createError.value = null
  creating.value = true
  try {
    await createEndpoint({
      name: form.value.name,
      target: form.value.target,
      endpoint_type: form.value.endpoint_type,
      interval: form.value.interval,
    })
    // HTTPS endpoints get an auto-detected cert monitor (source='auto')
    // at first check — it is tied to the endpoint and not counted against
    // the standalone cert quota.

    showCreateForm.value = false
    resetForm()
    store.fetchEndpoints()
    reload()
  } catch (e) {
    createError.value = e
  } finally {
    creating.value = false
  }
}

onMounted(() => {
  store.fetchEndpoints()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="p-3 sm:p-6">
    <div class="mx-auto max-w-7xl">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Endpoints</h1>
        <p class="mt-1 text-sm text-mnt-muted">
          HTTP/TCP endpoint health checks
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
          :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} endpoints` : ''"
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
          @click="showCreateForm = !showCreateForm; if (!showCreateForm) resetForm()"
        >
          {{ showCreateForm ? 'Cancel' : 'New Endpoint' }}
        </button>
      </div>
    </div>

    <FeatureHint
      storage-key="endpoints"
      title="Define HTTP/TCP checks with labels"
      :doc-href="docUrl('features/endpoints/#quick-start')"
    >
      Declare endpoints directly on your {{ isK8s ? 'pods' : 'containers' }} with
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">maintenant.endpoint.http</code>
      or
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">maintenant.endpoint.tcp</code>,
      and tune the interval, failure/recovery thresholds, expected status codes, or TLS verification via sibling {{ labelOrAnnotation }}s. Use indexed labels
      (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">maintenant.endpoint.0.*</code>)
      to monitor multiple endpoints from the same {{ isK8s ? 'pod' : 'container' }}.
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
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--mnt-text-primary)' }">Create Endpoint Monitor</h3>
      <QuotaRefusal v-if="createError" :error="createError" />
      <form class="flex flex-col gap-3" @submit.prevent="handleCreate">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Name</label>
            <input
              v-model="form.name"
              type="text"
              placeholder="e.g., Production API"
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
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Type</label>
            <div class="flex gap-2">
              <button
                v-for="t in (['http', 'tcp'] as const)"
                :key="t"
                type="button"
                class="flex-1 rounded-lg px-3 py-2 text-sm font-medium transition min-h-[44px]"
                :style="{
                  border: form.endpoint_type === t
                    ? '1px solid var(--mnt-accent)'
                    : '1px solid var(--mnt-border-default)',
                  backgroundColor: form.endpoint_type === t
                    ? 'var(--mnt-accent)'
                    : 'var(--mnt-bg-elevated)',
                  color: form.endpoint_type === t
                    ? 'var(--mnt-text-inverted)'
                    : 'var(--mnt-text-secondary)',
                  textTransform: 'uppercase',
                }"
                @click="form.endpoint_type = t"
              >
                {{ t }}
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">
            {{ form.endpoint_type === 'http' ? 'URL' : 'Host:Port' }}
          </label>
          <input
            v-model="form.target"
            type="text"
            :placeholder="form.endpoint_type === 'http' ? 'https://example.com/health' : 'db.example.com:5432'"
            :style="{
              width: '100%',
              borderRadius: 'var(--mnt-radius-md)',
              border: '1px solid var(--mnt-border-default)',
              backgroundColor: 'var(--mnt-bg-elevated)',
              color: 'var(--mnt-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              fontFamily: 'monospace',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Check Interval</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.interval === preset.value
                  ? '1px solid var(--mnt-accent)'
                  : '1px solid var(--mnt-border-default)',
                backgroundColor: form.interval === preset.value
                  ? 'var(--mnt-accent)'
                  : 'transparent',
                color: form.interval === preset.value
                  ? 'var(--mnt-text-inverted)'
                  : 'var(--mnt-text-secondary)',
              }"
              @click="form.interval = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>

        <button
          type="submit"
          :disabled="creating"
          :style="{
            alignSelf: 'flex-start',
            borderRadius: 'var(--mnt-radius-lg)',
            backgroundColor: 'var(--mnt-accent)',
            color: 'var(--mnt-text-inverted)',
            padding: '0.5rem 1rem',
            fontSize: '0.875rem',
            fontWeight: '500',
            opacity: creating ? '0.6' : '1',
          }"
        >
          {{ creating ? 'Creating...' : 'Create' }}
        </button>
      </form>
    </div>

    <!-- Config errors -->
    <InlineAlert
      v-if="store.configErrors.length > 0"
      severity="warning"
      :tag="`${store.configErrors.length} ${store.configErrors.length > 1 ? 'ERRORS' : 'ERROR'}`"
      class="mb-6 config-errors"
    >
      <template #title>Label configuration errors</template>
      <ul class="space-y-1">
        <li v-for="(err, i) in store.configErrors" :key="i" class="flex items-start gap-2">
          <span class="bullet mt-1.5 h-1 w-1 shrink-0 rounded-full" />
          <span>
            <strong class="font-semibold" style="color: var(--mnt-alert-warn-title)">{{ err.container_name }}</strong>
            <span class="mx-1 opacity-70">({{ err.label_key }})</span>
            <span>{{ err.error }}</span>
          </span>
        </li>
      </ul>
    </InlineAlert>

    <ListToolbar
      scope="endpoints"
      :search="store.searchQuery"
      :status="store.statusFilter"
      :chips="statusChips"
      :result-count="store.filteredEndpoints.length"
      :active-filter-count="store.activeFilterCount"
      search-placeholder="Search endpoints"
      @update:search="store.searchQuery = $event"
      @update:status="store.statusFilter = $event"
      @reset="store.resetFilters()"
    >
      <template #filters>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-muted">Type</span>
          <select
            v-model="store.typeFilter"
            class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-elevated px-2 text-sm text-mnt-secondary"
          >
            <option value="">All types</option>
            <option value="http">HTTP</option>
            <option value="tcp">TCP</option>
          </select>
        </label>

        <label class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-muted">Container</span>
          <select
            v-model="store.containerFilter"
            class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-elevated px-2 text-sm text-mnt-secondary"
          >
            <option value="">All containers</option>
            <option v-for="name in [...store.endpointsByContainer.keys()]" :key="name" :value="name">
              {{ name }}
            </option>
          </select>
        </label>

        <label class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-muted">Retired</span>
          <select
            v-model="store.showRetired"
            class="focus-ring min-h-[38px] rounded-lg border border-mnt-default bg-mnt-elevated px-2 text-sm text-mnt-secondary"
            @change="onRetiredChange"
          >
            <option :value="false">Live only</option>
            <option :value="true">Include retired</option>
          </select>
        </label>
      </template>
    </ListToolbar>

    <!-- Loading -->
    <LoadingSkeleton v-if="store.loading" variant="cards" :count="6" />

    <!-- Error -->
    <ErrorState v-else-if="store.error" :message="store.error" />

    <template v-else-if="store.filteredEndpoints.length > 0">
      <div v-if="view === 'cards'" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <EndpointCard
          v-for="ep in store.filteredEndpoints"
          :key="ep.id"
          :endpoint="ep"
          @select="openDetail('endpoint', ep.id)"
          @deleted="store.fetchEndpoints(); reload()"
        />
      </div>

      <div v-else-if="view === 'rows'" class="overflow-hidden rounded-xl border border-mnt-default">
        <EndpointRow
          v-for="ep in store.filteredEndpoints"
          :key="ep.id"
          :endpoint="ep"
          @select="openDetail('endpoint', ep.id)"
        />
      </div>

      <DataTable
        v-else
        :columns="columns"
        :rows="store.filteredEndpoints"
        :row-key="(ep: Endpoint) => ep.id"
        :sort-value="sortValue"
        :tone="endpointTone"
        default-sort="target"
        caption="Endpoints"
        @select="(ep: Endpoint) => openDetail('endpoint', ep.id)"
      >
        <template #cell-target="{ row }">
          <span class="font-medium text-mnt-primary">{{ row.target }}</span>
        </template>
        <template #cell-type="{ row }">
          <span class="font-mono text-xs uppercase">{{ row.endpoint_type }}</span>
        </template>
        <template #cell-source="{ row }">{{ sourceOf(row) }}</template>
        <template #cell-status="{ row }">
          <span
            v-if="row.stale"
            class="rounded-full bg-mnt-sev-unknown px-2 py-0.5 text-xs font-medium text-mnt-sev-unknown"
            :title="`Agent offline · last known: ${row.status}`"
          >offline</span>
          <EndpointStatusBadge v-else :status="row.status" />
        </template>
        <template #cell-response="{ row }">
          <span class="font-mono tabular-nums">{{ formatResponseTime(row.last_response_time_ms) }}</span>
        </template>
        <template #cell-interval="{ row }">
          <span class="font-mono text-xs">{{ row.config.interval }}</span>
        </template>
        <template #cell-last_check="{ row }">{{ timeAgo(row.last_check_at, 'never') }}</template>
      </DataTable>
    </template>

    <!-- No match for the current filters, as opposed to no endpoints at all -->
    <EmptyState
      v-else-if="hasFilters"
      :icon="Globe"
      title="No endpoint matches your filters"
      description="Try a different search term, or clear the filters to see every endpoint again."
    >
      <template #action>
        <button
          type="button"
          class="focus-ring min-h-[44px] rounded-lg border border-mnt-default px-4 text-sm font-medium text-mnt-secondary hover:text-mnt-primary"
          @click="store.resetFilters()"
        >
          Clear filters
        </button>
      </template>
    </EmptyState>

    <!-- Empty state -->
    <EmptyState
      v-else
      :icon="Globe"
      title="No endpoints yet"
      :description="`Add maintenant.endpoint ${labelOrAnnotation}s to your ${isK8s ? 'pods' : 'containers'}, or create a standalone monitor with the button above.`"
    />
  </div>
  </div>
</template>

<style scoped>
.config-errors .bullet {
  background: var(--mnt-alert-warn-dot);
  opacity: 0.7;
}
</style>
