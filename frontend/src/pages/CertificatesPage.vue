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
import { docUrl } from '@/utils/docs'

const store = useCertificatesStore()
const containers = useContainersStore()
const { openDetail } = inject(detailSlideOverKey)!
const { getQuota, reload } = useEdition()
const quota = getQuota('certificates')

const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')
const showCreateForm = ref(false)
const createError = ref<string | null>(null)

const isQuotaError = computed(() => {
  return createError.value?.includes('Upgrade to Pro') || false
})

const form = ref({
  hostname: '',
  port: 443,
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
    form.value = { hostname: '', port: 443, check_interval_seconds: 43200 }
    store.fetchCertificates()
    reload()
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to create certificate monitor'
  }
}

function handleSelect(id: number) {
  openDetail('certificate', id)
}
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-pb-primary">Certificates</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--pb-text-muted)' }">
          SSL/TLS certificate monitoring &amp; expiration alerts
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
          :title="quota.isAtLimit ? `Community edition limited to ${quota.limit} certificate monitors` : ''"
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
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.tls.certificates</code>.
      Alerts fire at 30, 14, 7, 3 and 1 day before expiry, plus on chain errors.
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
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">Create Certificate Monitor</h3>
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
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Hostname</label>
            <input
              v-model="form.hostname"
              type="text"
              placeholder="e.g., example.com"
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
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Port</label>
            <input
              v-model.number="form.port"
              type="number"
              min="1"
              max="65535"
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
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Check Interval</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.check_interval_seconds === preset.value
                  ? '1px solid var(--pb-accent)'
                  : '1px solid var(--pb-border-default)',
                backgroundColor: form.check_interval_seconds === preset.value
                  ? 'var(--pb-accent)'
                  : 'transparent',
                color: form.check_interval_seconds === preset.value
                  ? 'var(--pb-text-inverted)'
                  : 'var(--pb-text-secondary)',
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

    <!-- Status summary + filters -->
    <div class="mb-6 flex flex-wrap items-center gap-4 text-sm">
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-ok-bg)', color: 'var(--pb-status-ok)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.valid }} valid
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-warn-bg)', color: 'var(--pb-status-warn)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.expiring }} expiring
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-down-bg)', color: 'var(--pb-status-down)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.expired }} expired
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-status-critical-bg)', color: 'var(--pb-status-critical)', padding: '0.25rem 0.75rem' }">
        {{ store.statusCounts.error }} error
      </span>
      <span :style="{ borderRadius: '9999px', backgroundColor: 'var(--pb-bg-elevated)', color: 'var(--pb-text-muted)', padding: '0.25rem 0.75rem' }">
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
            ? '1px solid var(--pb-accent)'
            : '1px solid var(--pb-border-default)',
          backgroundColor: store.statusFilter === f.value
            ? 'var(--pb-accent)'
            : 'transparent',
          color: store.statusFilter === f.value
            ? 'var(--pb-text-inverted)'
            : 'var(--pb-text-secondary)',
        }"
        @click="store.statusFilter = f.value"
      >
        {{ f.label }}
      </button>
    </div>

    <!-- Loading -->
    <div v-if="store.loading" class="py-12 text-center" :style="{ color: 'var(--pb-text-muted)' }">
      Loading certificates...
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

    <!-- Content area with persistent background hint -->
    <div v-else class="relative min-h-[300px]">
      <!-- Background hint — always visible -->
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <svg width="56" height="56" viewBox="0 0 56 56" fill="none" stroke="currentColor" stroke-width="1.5" class="mb-4" style="color: var(--pb-text-muted)">
          <rect x="10" y="6" width="36" height="44" rx="4" />
          <path d="M20 20h16M20 28h16M20 36h10" stroke-linecap="round" />
          <circle cx="40" cy="40" r="10" fill="var(--pb-bg-primary)" />
          <path d="M37 40l2 2 4-4" stroke="var(--pb-status-ok)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <p class="text-sm mb-2 max-w-sm" :style="{ color: 'var(--pb-text-muted)' }">
          HTTPS endpoints are auto-detected from {{ labelOrAnnotation }}s. Create standalone monitors for additional hosts.
        </p>
        <p class="text-sm max-w-sm" :style="{ color: 'var(--pb-text-muted)' }">
          Add the <code class="rounded-md px-1.5 py-0.5 text-xs" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.tls.certificates</code>
          {{ labelOrAnnotation }} to monitor specific certificates.
        </p>
      </div>

      <!-- Certificate grid — overlays on top -->
      <div
        v-if="sortedCertificates.length > 0"
        class="absolute inset-0 grid gap-4 sm:grid-cols-2 lg:grid-cols-3 content-start"
        :style="{ backgroundColor: 'var(--pb-bg-primary)' }"
      >
        <CertificateCard
          v-for="cert in sortedCertificates"
          :key="cert.id"
          :certificate="cert"
          @refresh="store.fetchCertificates(); reload()"
          @select="handleSelect($event)"
        />
      </div>
    </div>
  </div>
  </div>
</template>
