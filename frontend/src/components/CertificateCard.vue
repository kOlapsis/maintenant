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
import { computed, ref } from 'vue'
import type { CertMonitor } from '@/services/certificateApi'
import { deleteCertificate } from '@/services/certificateApi'
import { useConfirm } from '@/composables/useConfirm'
import { timeAgo } from '@/utils/time'
import CertificateStatusBadge from './CertificateStatusBadge.vue'

const props = defineProps<{
  certificate: CertMonitor
}>()

const emit = defineEmits<{
  refresh: []
  select: [id: number]
}>()

function formatDaysRemaining(cert: CertMonitor): string {
  const days = cert.latest_check?.days_remaining
  if (days === undefined || days === null) return '-'
  if (days < 0) return 'Expired'
  if (days === 0) return 'Today'
  return `${days}d`
}

const formatTime = (iso: string | undefined) => timeAgo(iso, 'never')

function countdownColor(days: number | undefined | null): string {
  if (days === undefined || days === null) return 'var(--pb-text-muted)'
  if (days > 30) return 'var(--pb-status-ok)'
  if (days > 7) return 'var(--pb-status-warn)'
  if (days > 3) return 'var(--pb-status-critical)'
  return 'var(--pb-status-down)'
}

// Expiry progress bar: percentage of lifetime elapsed
const expiryProgress = computed(() => {
  const check = props.certificate.latest_check
  if (!check?.not_before || !check?.not_after) return null
  const start = new Date(check.not_before).getTime()
  const end = new Date(check.not_after).getTime()
  const now = Date.now()
  const total = end - start
  if (total <= 0) return 100
  const elapsed = now - start
  return Math.max(0, Math.min(100, (elapsed / total) * 100))
})

const confirm = useConfirm()
const deleting = ref(false)

async function handleDelete() {
  const ok = await confirm({
    title: 'Delete certificate monitor',
    message: `Remove the certificate monitor for "${props.certificate.hostname}:${props.certificate.port}"? This cannot be undone.`,
    confirmLabel: 'Delete',
    destructive: true,
  })
  if (!ok) return
  deleting.value = true
  try {
    await deleteCertificate(props.certificate.id)
    emit('refresh')
  } catch {
    // ignore - auto-detected monitors can't be deleted
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div
    class="cursor-pointer"
    :style="{
      backgroundColor: 'var(--pb-bg-surface)',
      border: '1px solid var(--pb-border-default)',
      borderRadius: 'var(--pb-radius-lg)',
      padding: '1rem',
      boxShadow: 'var(--pb-shadow-card)',
      transition: 'box-shadow 0.15s ease',
    }"
    @click="emit('select', certificate.id)"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0 flex-1">
        <h3 class="truncate text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">
          {{ certificate.hostname }}
        </h3>
        <p class="mt-0.5 text-xs" :style="{ color: 'var(--pb-text-muted)' }">
          :{{ certificate.port }}
        </p>
      </div>
      <div class="ml-2 flex items-center gap-1.5">
        <span
          class="inline-flex items-center rounded-full px-1.5 py-0.5 text-xs font-medium"
          :style="{
            backgroundColor: certificate.source === 'auto' ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-warn-bg)',
            color: certificate.source === 'auto' ? 'var(--pb-accent)' : 'var(--pb-status-warn)',
          }"
        >
          {{ certificate.source }}
        </span>
        <CertificateStatusBadge :status="certificate.status" />
      </div>
    </div>

    <!-- Expiry progress bar -->
    <div v-if="expiryProgress !== null" class="mt-3">
      <div
        class="h-1.5 w-full rounded-full"
        :style="{ backgroundColor: 'var(--pb-bg-elevated)' }"
      >
        <div
          class="h-1.5 rounded-full transition-all"
          :style="{
            width: expiryProgress + '%',
            backgroundColor: countdownColor(certificate.latest_check?.days_remaining),
          }"
        />
      </div>
    </div>

    <div class="mt-3 flex items-center justify-between text-xs" :style="{ color: 'var(--pb-text-muted)' }">
      <div class="flex items-center gap-3">
        <span v-if="certificate.latest_check?.issuer_cn">
          {{ certificate.latest_check.issuer_cn }}
        </span>
        <!-- Countdown badge -->
        <span
          class="rounded-full px-2 py-0.5 font-medium"
          :style="{
            color: countdownColor(certificate.latest_check?.days_remaining),
          }"
        >
          {{ formatDaysRemaining(certificate) }}
        </span>
      </div>
      <span>{{ formatTime(certificate.last_check_at) }}</span>
    </div>

    <!-- Actions -->
    <div
      v-if="certificate.source === 'standalone'"
      class="mt-3 flex items-center pt-2"
      :style="{ borderTop: '1px solid var(--pb-border-subtle)' }"
      @click.stop
    >
      <button
        class="ml-auto rounded px-2 py-0.5 text-xs transition hover:opacity-80"
        :style="{ color: 'var(--pb-status-down)' }"
        :disabled="deleting"
        @click="handleDelete"
      >
        {{ deleting ? 'Deleting...' : 'Delete' }}
      </button>
    </div>
  </div>
</template>
