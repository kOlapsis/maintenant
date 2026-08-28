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
import { useEdition } from '@/composables/useEdition'
import { createCertificate } from '@/services/certificateApi'
import CertificateCard from '@/components/CertificateCard.vue'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import LoadingSkeleton from '@/components/ui/LoadingSkeleton.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import ErrorState from '@/components/ui/ErrorState.vue'
import { ShieldCheck } from 'lucide-vue-next'
import { docUrl } from '@/utils/docs'
import QuotaRefusal from '@/components/QuotaRefusal.vue'

const store = useCertificatesStore()
const containers = useContainersStore()
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

const statusFilters = [
  { label: 'All', value: '' },
  { label: 'Valid', value: 'valid' },
  { label: 'Expiring', value: 'expiring' },
  { label: 'Expired', value: 'expired' },
  { label: 'Error', value: 'error' },
  { label: 'Unknown', value: 'unknown' },
] as const

// Sort certificates by days_remaining ascending (most urgent first)
const sortedCertificates = computed(() => {
  return [...store.filteredCertificates].sort((a, b) => {
    const daysA = a.latest_check?.days_remaining ?? 999999
    const daysB = b.latest_check?.days_remaining ?? 999999
    return daysA - daysB
  })
})

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
  <div class="overflow-y-auto p-3 sm:p-6">
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

    <!-- Status summary + filters -->
    <div class="mb-6 flex flex-wrap items-center gap-4 text-sm">
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-ok-bg)', color: 'var(--mnt-status-ok)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.valid }} valid
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-warn-bg)', color: 'var(--mnt-status-warn)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.expiring }} expiring
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-down-bg)', color: 'var(--mnt-status-down)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.expired }} expired
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-status-critical-bg)', color: 'var(--mnt-status-critical)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.error }} error
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--mnt-bg-elevated)', color: 'var(--mnt-text-muted)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.unknown }} unknown
      </span>
    </div>

    <!-- Filter bar -->
    <div class="mb-4 flex gap-2">
      <button
        v-for="f in statusFilters"
        :key="f.value"
        class="rounded-full px-3 py-1 text-xs font-medium transition"
        :style="{
          border: store.statusFilter === f.value
            ? '1px solid var(--mnt-accent)'
            : '1px solid var(--mnt-border-default)',
          backgroundColor: store.statusFilter === f.value
            ? 'var(--mnt-accent)'
            : 'transparent',
          color: store.statusFilter === f.value
            ? 'var(--mnt-text-inverted)'
            : 'var(--mnt-text-secondary)',
        }"
        @click="store.statusFilter = f.value"
      >
        {{ f.label }}
      </button>
    </div>

    <!-- Loading -->
    <LoadingSkeleton v-if="store.loading" variant="cards" :count="6" />

    <!-- Error -->
    <ErrorState v-else-if="store.error" :message="store.error" />

    <!-- Certificate grid -->
    <div
      v-else-if="sortedCertificates.length > 0"
      class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
    >
      <CertificateCard
        v-for="cert in sortedCertificates"
        :key="cert.id"
        :certificate="cert"
        @refresh="store.fetchCertificates(); reload()"
        @select="handleSelect($event)"
      />
    </div>

    <!-- Empty state -->
    <EmptyState
      v-else
      :icon="ShieldCheck"
      title="No certificates monitored"
      :description="`HTTPS endpoints are auto-detected from ${labelOrAnnotation}s. Add the maintenant.tls.certificates ${labelOrAnnotation} or create a standalone monitor above.`"
    />
  </div>
  </div>
</template>
