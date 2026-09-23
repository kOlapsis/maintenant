<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
import CountBadge from '@/components/ui/CountBadge.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FormField from '@/components/ui/FormField.vue'
import TextInput from '@/components/ui/TextInput.vue'
import SelectInput from '@/components/ui/SelectInput.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'

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
          :title="quota.isAtLimit ? `Your edition is limited to ${quota.limit} certificate monitors` : ''"
          @click="showCreateForm = !showCreateForm"
        >
          {{ showCreateForm ? 'Cancel' : 'New Monitor' }}
        </UiButton>
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
          <FormField label="Hostname">
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                v-model="form.hostname"
                placeholder="e.g., example.com"
                :aria-describedby="describedBy"
                :invalid="invalid"
                required
              />
            </template>
          </FormField>
          <FormField label="Port">
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                :model-value="form.port"
                type="number"
                min="1"
                max="65535"
                :aria-describedby="describedBy"
                :invalid="invalid"
                @update:model-value="(v) => (form.port = typeof v === 'number' ? v : 443)"
              />
            </template>
          </FormField>
          <FormField
            label="Server name (SNI) — optional"
            hint="Sent as SNI during the TLS handshake; the certificate is validated against this name instead of the hostname. Useful to verify which certificate a reverse proxy serves for a given virtual host (failover / keepalived setups)."
            class="sm:col-span-2"
          >
            <template #default="{ id, describedBy, invalid }">
              <TextInput
                :id="id"
                v-model="form.server_name"
                placeholder="e.g., service.example.com"
                :aria-describedby="describedBy"
                :invalid="invalid"
              />
            </template>
          </FormField>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--mnt-text-secondary)' }">Check Interval</label>
          <SegmentedToggle
            :model-value="String(form.check_interval_seconds)"
            :options="intervalPresets.map((p) => ({ value: String(p.value), label: p.label }))"
            ariaLabel="Check interval"
            @update:model-value="(v) => (form.check_interval_seconds = Number(v))"
          />
        </div>
        <UiButton type="submit" variant="primary" class="self-start">
          Create
        </UiButton>
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
          <SelectInput
            v-model="sourceFilter"
            size="sm"
            :options="[
              { value: '', label: 'Any source' },
              { value: 'auto', label: 'Auto-detected' },
              { value: 'standalone', label: 'Standalone' },
            ]"
          />
        </label>

        <label v-if="issuers.length > 0" class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Issuer</span>
          <SelectInput
            v-model="issuerFilter"
            size="sm"
            :options="[{ value: '', label: 'Any issuer' }, ...issuers.map((issuer) => ({ value: issuer, label: issuer }))]"
          />
        </label>

        <label v-if="view !== 'table'" class="flex flex-col gap-1">
          <span class="text-xs font-semibold text-mnt-secondary">Sort by</span>
          <SelectInput
            v-model="sortBy"
            size="sm"
            :options="[
              { value: 'expiry', label: 'Expiring first' },
              { value: 'hostname', label: 'Domain (A-Z)' },
              { value: 'checked', label: 'Last check' },
            ]"
          />
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
        <UiButton variant="secondary" @click="resetFilters">
          Clear filters
        </UiButton>
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
