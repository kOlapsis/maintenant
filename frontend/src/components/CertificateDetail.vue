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
import { ref, watch, onMounted, computed } from 'vue'
import { getCertificate, type CertificateDetailResponse, type CertChainEntry } from '@/services/certificateApi'
import CertificateStatusBadge from './CertificateStatusBadge.vue'

const props = defineProps<{
  certificateId: number
}>()

const emit = defineEmits<{
  close: []
}>()

const detail = ref<CertificateDetailResponse | null>(null)
const loading = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = null
  try {
    detail.value = await getCertificate(props.certificateId)
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load certificate details'
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => props.certificateId, load)

function formatDate(iso: string | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function chainStatusStyle(entry: CertChainEntry): Record<string, string> {
  const now = new Date()
  const notAfter = new Date(entry.not_after)
  if (notAfter < now) {
    return {
      borderColor: 'var(--pb-status-down)',
      backgroundColor: 'var(--pb-status-down-bg)',
    }
  }
  return {
    borderColor: 'var(--pb-status-ok)',
    backgroundColor: 'var(--pb-status-ok-bg)',
  }
}

// Expiry progress bar calculation
const expiryProgress = computed(() => {
  if (!detail.value?.latest_check?.not_before || !detail.value?.latest_check?.not_after) return null
  const start = new Date(detail.value.latest_check.not_before).getTime()
  const end = new Date(detail.value.latest_check.not_after).getTime()
  const now = Date.now()
  const total = end - start
  if (total <= 0) return 100
  const elapsed = now - start
  return Math.max(0, Math.min(100, (elapsed / total) * 100))
})

function countdownColor(days: number | undefined): string {
  if (days === undefined || days === null) return 'var(--pb-text-muted)'
  if (days > 30) return 'var(--pb-status-ok)'
  if (days > 7) return 'var(--pb-status-warn)'
  if (days > 3) return 'var(--pb-status-critical)'
  return 'var(--pb-status-down)'
}

function countdownBgColor(days: number | undefined): string {
  if (days === undefined || days === null) return 'var(--pb-bg-elevated)'
  if (days > 30) return 'var(--pb-status-ok-bg)'
  if (days > 7) return 'var(--pb-status-warn-bg)'
  if (days > 3) return 'var(--pb-status-critical-bg)'
  return 'var(--pb-status-down-bg)'
}
</script>

<template>
  <div class="h-full overflow-y-auto p-5">
    <div v-if="loading" class="py-8 text-center" :style="{ color: 'var(--pb-text-muted)' }">Loading...</div>
    <div
      v-else-if="error"
      class="rounded p-3 text-sm"
      :style="{
        backgroundColor: 'var(--pb-status-down-bg)',
        color: 'var(--pb-status-down)',
        borderRadius: 'var(--pb-radius-sm)',
      }"
    >
      {{ error }}
    </div>
    <div v-else-if="detail">
      <!-- Monitor info -->
      <div class="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2">
        <span class="text-lg font-medium break-all" :style="{ color: 'var(--pb-text-primary)' }">
          {{ detail.certificate.hostname }}:{{ detail.certificate.port }}
        </span>
        <CertificateStatusBadge :status="detail.certificate.status" />
        <span
          class="rounded-full px-2 py-0.5 text-xs font-medium"
          :style="{
            backgroundColor: detail.certificate.source === 'auto' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-warn-bg)',
            color: detail.certificate.source === 'auto' ? 'var(--pb-accent)' : 'var(--pb-status-warn)',
          }"
        >
          {{ detail.certificate.source }}
        </span>
      </div>

      <!-- Days remaining countdown badge -->
      <div
        v-if="detail.latest_check"
        class="mb-4 inline-flex items-center gap-2 rounded-full px-3 py-1.5"
        :style="{
          backgroundColor: countdownBgColor(detail.latest_check.days_remaining),
          color: countdownColor(detail.latest_check.days_remaining),
          fontWeight: '600',
          fontSize: '0.875rem',
        }"
      >
        <span v-if="detail.latest_check.days_remaining !== undefined && detail.latest_check.days_remaining !== null">
          {{ detail.latest_check.days_remaining }} days remaining
        </span>
        <span v-else>Unknown</span>
      </div>

      <!-- Expiry progress bar -->
      <div v-if="expiryProgress !== null" class="mb-4">
        <div class="mb-1 flex justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
          <span>Issued</span>
          <span>Expires</span>
        </div>
        <div
          class="h-2 w-full rounded-full"
          :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
        >
          <div
            class="h-2 rounded-full transition-all"
            :style="{
              width: expiryProgress + '%',
              backgroundColor: countdownColor(detail.latest_check?.days_remaining),
            }"
          />
        </div>
      </div>

      <!-- Certificate fields -->
      <div v-if="detail.latest_check" class="mb-4 grid gap-3 grid-cols-1 sm:grid-cols-2">
        <div class="min-w-0">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Subject CN</span>
          <p class="text-sm break-words" :style="{ color: 'var(--pb-text-primary)' }">{{ detail.latest_check.subject_cn || '-' }}</p>
        </div>
        <div class="min-w-0">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Issuer</span>
          <p class="text-sm break-words" :style="{ color: 'var(--pb-text-primary)' }">
            {{ detail.latest_check.issuer_cn }}
            <span v-if="detail.latest_check.issuer_org" :style="{ color: 'var(--pb-text-muted)' }">
              ({{ detail.latest_check.issuer_org }})
            </span>
          </p>
        </div>
        <div class="min-w-0 sm:col-span-2">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">SANs</span>
          <p class="text-sm break-words" :style="{ color: 'var(--pb-text-primary)' }">
            {{ detail.latest_check.sans?.join(', ') || '-' }}
          </p>
        </div>
        <div class="min-w-0 sm:col-span-2">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Serial Number</span>
          <p class="break-all font-mono text-sm" :style="{ color: 'var(--pb-text-primary)' }">{{ detail.latest_check.serial_number || '-' }}</p>
        </div>
        <div class="min-w-0">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Signature Algorithm</span>
          <p class="text-sm break-words" :style="{ color: 'var(--pb-text-primary)' }">{{ detail.latest_check.signature_algorithm || '-' }}</p>
        </div>
        <div class="min-w-0">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Valid From</span>
          <p class="text-sm" :style="{ color: 'var(--pb-text-primary)' }">{{ formatDate(detail.latest_check.not_before) }}</p>
        </div>
        <div class="min-w-0">
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Valid Until</span>
          <p class="text-sm" :style="{ color: 'var(--pb-text-primary)' }">{{ formatDate(detail.latest_check.not_after) }}</p>
        </div>
        <div>
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Chain Valid</span>
          <p class="text-sm">
            <span v-if="detail.latest_check.chain_valid" :style="{ color: 'var(--pb-status-ok)' }">Yes</span>
            <span v-else :style="{ color: 'var(--pb-status-down)' }">
              No{{ detail.latest_check.chain_error ? `: ${detail.latest_check.chain_error}` : '' }}
            </span>
          </p>
        </div>
        <div>
          <span class="text-xs font-medium" :style="{ color: 'var(--pb-text-muted)' }">Hostname Match</span>
          <p class="text-sm">
            <span v-if="detail.latest_check.hostname_match" :style="{ color: 'var(--pb-status-ok)' }">Yes</span>
            <span v-else :style="{ color: 'var(--pb-status-down)' }">No</span>
          </p>
        </div>
      </div>

      <div
        v-if="detail.latest_check?.error_message"
        class="mb-4 rounded p-3 text-sm"
        :style="{
          backgroundColor: 'var(--pb-status-down-bg)',
          color: 'var(--pb-status-down)',
          borderRadius: 'var(--pb-radius-sm)',
        }"
      >
        {{ detail.latest_check.error_message }}
      </div>

      <!-- Chain visualization -->
      <div v-if="detail.latest_check?.chain?.length" class="mt-4">
        <h4 class="mb-2 text-sm font-semibold" :style="{ color: 'var(--pb-text-secondary)' }">Certificate Chain</h4>
        <div class="space-y-2">
          <div
            v-for="entry in detail.latest_check.chain"
            :key="entry.position"
            class="rounded-lg p-3"
            :style="{
              border: '1px solid',
              ...chainStatusStyle(entry),
              borderRadius: 'var(--pb-radius-md)',
            }"
          >
            <div class="flex items-center justify-between">
              <div>
                <span class="text-xs" :style="{ color: 'var(--pb-text-muted)' }">#{{ entry.position }}</span>
                <span class="ml-2 text-sm font-medium" :style="{ color: 'var(--pb-text-primary)' }">{{ entry.subject_cn }}</span>
              </div>
              <span class="text-xs" :style="{ color: 'var(--pb-text-muted)' }">Issued by: {{ entry.issuer_cn }}</span>
            </div>
            <div class="mt-1 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
              {{ formatDate(entry.not_before) }} &mdash; {{ formatDate(entry.not_after) }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
