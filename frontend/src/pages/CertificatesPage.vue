<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useCertificatesStore } from '@/stores/certificates'
import { useContainersStore } from '@/stores/containers'
import { createCertificate } from '@/services/certificateApi'
import CertificateCard from '@/components/CertificateCard.vue'
import CertificateDetail from '@/components/CertificateDetail.vue'

const store = useCertificatesStore()
const containers = useContainersStore()
const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')
const selectedId = ref<number | null>(null)
const showCreateForm = ref(false)
const createError = ref<string | null>(null)

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
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to create certificate monitor'
  }
}

function openDetail(id: number) {
  selectedId.value = id
}

function closeDetail() {
  selectedId.value = null
}
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-white">Certificates</h1>
        <p class="mt-1 text-sm" :style="{ color: 'var(--pb-text-muted)' }">
          SSL/TLS certificate monitoring &amp; expiration alerts
        </p>
      </div>
      <button
        class="min-h-[44px]"
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
        {{ showCreateForm ? 'Cancel' : 'New Monitor' }}
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
        {{ createError }}
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

    <!-- Slide-over detail -->
    <CertificateDetail
      v-if="selectedId"
      :certificate-id="selectedId"
      @close="closeDetail"
    />

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
          @refresh="store.fetchCertificates()"
          @select="openDetail($event)"
        />
      </div>
    </div>
  </div>
</template>
