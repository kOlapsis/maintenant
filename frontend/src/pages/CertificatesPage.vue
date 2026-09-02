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
import { useCertificatesStore } from '@/stores/certificates'
import { useContainersStore } from '@/stores/containers'
import { usePreferencesStore } from '@/stores/preferences'
import { useEdition } from '@/composables/useEdition'
import { useListFilter } from '@/composables/useListFilter'
import { createCertificate, type CertMonitor, type CertSource } from '@/services/certificateApi'
import CertificateCard from '@/components/CertificateCard.vue'
import CertificateRow from '@/components/CertificateRow.vue'
import CertificateStatusBadge from '@/components/CertificateStatusBadge.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import ListToolbar from '@/components/ui/ListToolbar.vue'
import DataTable, { type Column } from '@/components/ui/DataTable.vue'
import type { StatusChip } from '@/components/ui/listFilters'
import { certificateTone, countdownColor, formatDaysRemaining, formatExpiryDate } from '@/utils/certFormat'
import { timeAgo } from '@/utils/time'
import { ShieldCheck } from 'lucide-vue-next'
import { docUrl } from '@/utils/docs'
import QuotaRefusal from '@/components/QuotaRefusal.vue'

const store = useCertificatesStore()
const containers = useContainersStore()
const prefs = usePreferencesStore()
const { openDetail } = inject(detailSlideOverKey)!
const { getQuota, reload } = useEdition()
const quota = getQuota('certificates')

const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')
const showCreateForm = ref(false)
const createError = ref<unknown>(null)


const form = ref({
  hostname: '',
  port: 443,
  server_name: '',
  check_interval_seconds: 43200,
})

const intervalPresets = [
  { label: '1h', value: 3600 },
  { label: '6h', value: 21600 },
  { label: '12h', value: 43200 },
  { label: '24h', value: 86400 },
  { label: '7d', value: 604800 },
]

const view = computed(() => prefs.listView('certificates'))

const sourceFilter = ref<CertSource | ''>('')
const issuerFilter = ref('')
const sortBy = ref<'expiry' | 'hostname' | 'checked'>('expiry')

const issuers = computed(() => {
  const seen = new Set<string>()
  for (const cert of store.certificates) {
    const issuer = cert.latest_check?.issuer_cn
    if (issuer) seen.add(issuer)
  }
  return [...seen].sort((a, b) => a.localeCompare(b))
})

const {
  search,
  status,
  filtered,
  statusCounts,
  activeFilterCount,
  reset: resetSearchAndStatus,
} = useListFilter<CertMonitor>(
  computed(() => store.certificates),
  {
    searchFields: (c) => [
      c.hostname,
      c.server_name,
      c.latest_check?.subject_cn,
      c.latest_check?.issuer_cn,
      c.latest_check?.issuer_org,
    ],
    status: (c) => c.status,
    extra: {
      source: computed(() =>
        sourceFilter.value ? (c: CertMonitor) => c.source === sourceFilter.value : null,
      ),
      issuer: computed(() =>
        issuerFilter.value
          ? (c: CertMonitor) => c.latest_check?.issuer_cn === issuerFilter.value
          : null,
      ),
    },
  },
)

// Counts come from the list the other filters already narrowed, so a chip
// promising "2 expiring" really leaves 2 rows standing once it is clicked.
const chips = computed<StatusChip[]>(() =>
  ([
    ['valid', 'ok'],
    ['expiring', 'warn'],
    ['expired', 'down'],
    ['error', 'critical'],
    ['unknown', 'unknown'],
  ] as const).map(([value, tone]) => ({
    value,
    label: value,
    count: statusCounts.value.get(value) ?? 0,
    tone,
  })),
)

// Cards and rows keep an explicit sort; the table sorts through its own headers.
const sortedCertificates = computed(() => {
  const list = [...filtered.value]
  if (sortBy.value === 'hostname') {
    return list.sort((a, b) => a.hostname.localeCompare(b.hostname))
  }
  if (sortBy.value === 'checked') {
    return list.sort((a, b) => checkedAt(b) - checkedAt(a))
  }
  return list.sort((a, b) => daysLeft(a) - daysLeft(b))
})

function daysLeft(cert: CertMonitor): number {
  return cert.latest_check?.days_remaining ?? Number.MAX_SAFE_INTEGER
}

function checkedAt(cert: CertMonitor): number {
  return cert.last_check_at ? new Date(cert.last_check_at).getTime() : 0
}

const columns: Column[] = [
  { key: 'hostname', label: 'Domain', sortable: true, width: 'minmax(0, 2fr)' },
  { key: 'issuer', label: 'Issuer', priority: 'md' },
  { key: 'status', label: 'Status', sortable: true, width: '110px' },
  { key: 'days', label: 'Days left', sortable: true, align: 'right', width: '90px' },
  { key: 'expires', label: 'Expires', sortable: true, align: 'right', width: '130px', priority: 'sm' },
  { key: 'checked', label: 'Last check', align: 'right', width: '110px', priority: 'lg' },
]

function sortValue(cert: CertMonitor, key: string): string | number | undefined {
  switch (key) {
    case 'hostname':
      return cert.hostname
    case 'issuer':
      return cert.latest_check?.issuer_cn
    case 'status':
      return cert.status
    case 'days':
      return cert.latest_check?.days_remaining
    case 'expires':
      return cert.latest_check?.not_after
        ? new Date(cert.latest_check.not_after).getTime()
        : undefined
    case 'checked':
      return cert.last_check_at ? new Date(cert.last_check_at).getTime() : undefined
    default:
      return undefined
  }
}

function resetFilters() {
  resetSearchAndStatus()
  sourceFilter.value = ''
  issuerFilter.value = ''
}

onMounted(() => {
  store.fetchCertificates()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})

async function handleCreate() {
  createError.value = null
  try {
    await createCertificate(form.value)
    showCreateForm.value = false
    form.value = { hostname: '', port: 443, server_name: '', check_interval_seconds: 43200 }
    store.fetchCertificates()
    reload()
  } catch (e) {
    createError.value = e
  }
}

function handleSelect(id: string) {
  openDetail('certificate', id)
}
</script>

<template>
  <div class="p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Certificates</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--mnt-text-muted)' }">
          SSL/TLS certificate monitoring &amp; expiration alerts
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
          :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} certificate monitors` : ''"
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
          {{ showCreateForm ? 'Cancel' : 'New Monitor' }}
        </button>
      </div>
    </div>

    <FeatureHint
      storage-key="certificates"
      title="TLS expiry tracking, automatic and standalone"
      :doc-href="docUrl('features/certificates/#alert-thresholds')"
    >
      Any HTTPS endpoint {{ labelOrAnnotation }} auto-creates a certificate monitor &mdash; the full chain (leaf, intermediates, root) is validated on each check. Add standalone monitors for domains outside your stack, or declare extras with
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">maintenant.tls.certificates</code>.
      Alerts fire at 30, 14, 7, 3 and 1 day before expiry, plus on chain errors.
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
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--mnt-text-primary)' }">Create Certificate Monitor</h3>
      <QuotaRefusal v-if="createError" :error="createError" />
      <form class="flex flex-col gap-3" @submit.prevent="handleCreate">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Hostname</label>
            <input
              v-model="form.hostname"
              type="text"
              placeholder="e.g., example.com"
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
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Port</label>
            <input
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
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
          <div class="sm:col-span-2">
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">
              Server name (SNI) <span :style="{ color: 'var(--mnt-text-muted)' }">— optional</span>
            </label>
            <input
              v-model="form.server_name"
              type="text"
              placeholder="e.g., service.example.com"
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
            <p class="mt-1 text-xs" :style="{ color: 'var(--mnt-text-muted)' }">
              Sent as SNI during the TLS handshake; the certificate is validated against this name
              instead of the hostname. Useful to verify which certificate a reverse proxy serves
              for a given virtual host (failover / keepalived setups).
            </p>
          </div>
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
                border: form.check_interval_seconds === preset.value
                  ? '1px solid var(--mnt-accent)'
                  : '1px solid var(--mnt-border-default)',
                backgroundColor: form.check_interval_seconds === preset.value
                  ? 'var(--mnt-accent)'
                  : 'transparent',
                color: form.check_interval_seconds === preset.value
                  ? 'var(--mnt-text-inverted)'
                  : 'var(--mnt-text-secondary)',
              }"
              @click="form.check_interval_seconds = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
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

    <ListToolbar
      scope="certificates"
      :search="search"
      :status="status"
      :chips="chips"
      :result-count="filtered.length"
      :active-filter-count="activeFilterCount"
      search-placeholder="Search domain, CN or issuer"
      @update:search="search = $event"
      @update:status="status = $event"
      @reset="resetFilters"
    >
      <template #filters>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Source</span>
          <select v-model="sourceFilter" class="filter-select focus-ring">
            <option value="">Any source</option>
            <option value="auto">Auto-detected</option>
            <option value="standalone">Standalone</option>
          </select>
        </label>

        <label v-if="issuers.length > 0" class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Issuer</span>
          <select v-model="issuerFilter" class="filter-select focus-ring">
            <option value="">Any issuer</option>
            <option v-for="issuer in issuers" :key="issuer" :value="issuer">{{ issuer }}</option>
          </select>
        </label>

        <label v-if="view !== 'table'" class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Sort by</span>
          <select v-model="sortBy" class="filter-select focus-ring">
            <option value="expiry">Expiring first</option>
            <option value="hostname">Domain (A-Z)</option>
            <option value="checked">Last check</option>
          </select>
        </label>
      </template>
    </ListToolbar>

    <!-- Loading -->
    <LoadingSkeleton v-if="store.loading" variant="cards" :count="6" />

    <!-- Error -->
    <ErrorState v-else-if="store.error" :message="store.error" />

    <!-- Empty state -->
    <EmptyState
      v-else-if="store.certificates.length === 0"
      :icon="ShieldCheck"
      title="No certificates monitored"
      :description="`HTTPS endpoints are auto-detected from ${labelOrAnnotation}s. Add the maintenant.tls.certificates ${labelOrAnnotation} or create a standalone monitor above.`"
    />

    <!-- No match for the current search and filters -->
    <EmptyState
      v-else-if="filtered.length === 0"
      :icon="ShieldCheck"
      title="No certificate matches your filters"
      description="Try a different search term, or clear the filters to see every monitored certificate."
    >
      <template #action>
        <button
          class="focus-ring min-h-[44px] rounded-lg border border-mnt-default px-4 text-sm font-medium text-mnt-secondary hover:text-mnt-primary"
          @click="resetFilters"
        >
          Clear filters
        </button>
      </template>
    </EmptyState>

    <!-- Cards -->
    <div v-else-if="view === 'cards'" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <CertificateCard
        v-for="cert in sortedCertificates"
        :key="cert.id"
        :certificate="cert"
        @refresh="store.fetchCertificates(); reload()"
        @select="handleSelect($event)"
      />
    </div>

    <!-- Rows -->
    <div
      v-else-if="view === 'rows'"
      class="overflow-hidden rounded-xl border border-mnt-default bg-mnt-surface"
    >
      <CertificateRow
        v-for="cert in sortedCertificates"
        :key="cert.id"
        :certificate="cert"
        @select="handleSelect($event)"
      />
    </div>

    <!-- Table -->
    <DataTable
      v-else
      :columns="columns"
      :rows="filtered"
      :row-key="(cert: CertMonitor) => cert.id"
      :sort-value="sortValue"
      :tone="(cert: CertMonitor) => certificateTone(cert.status)"
      default-sort="days"
      caption="Monitored TLS certificates"
      @select="handleSelect($event.id)"
    >
      <template #cell-hostname="{ row }">
        <span class="font-medium text-mnt-primary">{{ row.hostname }}</span>
        <span class="text-mnt-muted">:{{ row.port }}</span>
      </template>
      <template #cell-issuer="{ row }">
        {{ row.latest_check?.issuer_cn || '-' }}
      </template>
      <template #cell-status="{ row }">
        <CertificateStatusBadge :status="row.status" />
      </template>
      <template #cell-days="{ row }">
        <span
          class="font-mono font-semibold tabular-nums"
          :style="{ color: countdownColor(row.latest_check?.days_remaining) }"
        >
          {{ formatDaysRemaining(row.latest_check?.days_remaining) }}
        </span>
      </template>
      <template #cell-expires="{ row }">
        {{ formatExpiryDate(row.latest_check?.not_after) }}
      </template>
      <template #cell-checked="{ row }">
        {{ timeAgo(row.last_check_at, 'never') }}
      </template>
    </DataTable>
  </div>
  </div>
</template>

<style scoped>
.filter-select {
  min-height: 38px;
  width: 100%;
  border: 1px solid var(--mnt-border-default);
  border-radius: var(--mnt-radius-md);
  background: var(--mnt-bg-elevated);
  color: var(--mnt-text-primary);
  padding: 0 0.5rem;
  font-size: 0.8125rem;
}
</style>
