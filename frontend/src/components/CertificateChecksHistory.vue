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
import { ref, watch, onMounted } from 'vue'
import { listChecks, type CertCheckResult } from '@/services/certificateApi'
import OCSPStatusBadge from './OCSPStatusBadge.vue'

const props = defineProps<{
  certificateId: number
}>()

const checks = ref<CertCheckResult[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = null
  try {
    const resp = await listChecks(props.certificateId, { limit: 25 })
    checks.value = resp.checks
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load check history'
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => props.certificateId, load)

function formatDateShort(iso: string | undefined): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function checkStatus(check: CertCheckResult): { color: string; label: string } {
  if (check.error_message) {
    return { color: 'var(--pb-status-down)', label: 'error' }
  }
  if (check.chain_valid === false || check.hostname_match === false) {
    return { color: 'var(--pb-status-down)', label: 'invalid' }
  }
  if (check.ocsp_status === 'revoked') {
    return { color: 'var(--pb-status-down)', label: 'revoked' }
  }
  const days = check.days_remaining
  if (days === undefined || days === null) {
    return { color: 'var(--pb-text-muted)', label: 'unknown' }
  }
  if (days <= 0) return { color: 'var(--pb-status-down)', label: 'expired' }
  if (days <= 7) return { color: 'var(--pb-status-critical)', label: 'expiring' }
  if (days <= 30) return { color: 'var(--pb-status-warn)', label: 'expiring' }
  return { color: 'var(--pb-status-ok)', label: 'valid' }
}
</script>

<template>
  <div>
    <div v-if="loading" class="py-8 text-center text-sm" :style="{ color: 'var(--pb-text-muted)' }">Loading...</div>
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
    <div v-else-if="checks.length === 0" class="py-8 text-center text-sm" :style="{ color: 'var(--pb-text-muted)' }">
      No check history yet.
    </div>
    <div v-else>
      <div
        v-for="check in checks"
        :key="check.id"
        class="flex items-center gap-3 px-2 py-1.5 text-sm"
        :style="{ borderBottom: '1px solid var(--pb-border-subtle)' }"
      >
        <span
          class="h-2 w-2 shrink-0 rounded-full"
          :style="{ backgroundColor: checkStatus(check).color }"
          :title="checkStatus(check).label"
        />
        <span class="shrink-0 tabular-nums" :style="{ color: 'var(--pb-text-secondary)' }">
          {{ formatDateShort(check.checked_at) }}
        </span>
        <span
          v-if="check.days_remaining !== undefined && check.days_remaining !== null && !check.error_message"
          class="shrink-0 tabular-nums text-xs"
          :style="{ color: 'var(--pb-text-muted)' }"
        >
          {{ check.days_remaining }}d
        </span>
        <span
          v-if="check.error_message"
          class="truncate text-xs"
          :style="{ color: 'var(--pb-status-down)' }"
        >
          {{ check.error_message }}
        </span>
        <div class="ml-auto">
          <OCSPStatusBadge v-if="check.ocsp_stapled && check.ocsp_status" :status="check.ocsp_status" />
        </div>
      </div>
    </div>
  </div>
</template>
