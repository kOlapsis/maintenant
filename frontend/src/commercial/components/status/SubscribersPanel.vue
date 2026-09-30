<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { useStatusAdminStore } from '@/stores/statusAdmin'
import { useEdition } from '@/composables/useEdition'
import SmtpNotConfigured from '@/components/SmtpNotConfigured.vue'

const store = useStatusAdminStore()
const { hasFeature } = useEdition()
</script>

<template>
  <div
    class="rounded-lg border p-6"
    style="background: var(--mnt-bg-surface); border-color: var(--mnt-border-default)"
  >
    <h2 class="mb-3 text-lg font-semibold" style="color: var(--mnt-text-primary)">Subscribers</h2>
    <SmtpNotConfigured
      v-if="!hasFeature('smtp')"
      class="mb-3"
      title="Visitors cannot subscribe until SMTP is configured"
    />
    <div class="mb-3 flex gap-4">
      <div class="rounded-lg border px-4 py-2" style="border-color: var(--mnt-border-default); background: var(--mnt-bg-elevated)">
        <p class="text-2xl font-bold" style="color: var(--mnt-text-primary)">{{ store.subscriberTotal }}</p>
        <p class="text-xs" style="color: var(--mnt-text-muted)">Total</p>
      </div>
      <div class="rounded-lg border px-4 py-2" style="border-color: var(--mnt-border-default); background: var(--mnt-bg-elevated)">
        <p class="text-2xl font-bold" style="color: var(--mnt-status-ok)">{{ store.subscriberConfirmed }}</p>
        <p class="text-xs" style="color: var(--mnt-text-muted)">Confirmed</p>
      </div>
    </div>
    <div v-if="(store.subscribers?.length ?? 0) === 0" class="text-center">
      <p class="text-sm" style="color: var(--mnt-text-muted)">No subscribers yet</p>
    </div>
    <div v-else class="space-y-1">
      <div
        v-for="sub in store.subscribers"
        :key="sub.id"
        class="flex items-center justify-between rounded px-3 py-1.5 text-sm transition-colors"
        style="color: var(--mnt-text-secondary)"
        @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--mnt-bg-hover)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
      >
        <span>{{ sub.email }}</span>
        <span
          class="rounded px-1.5 py-0.5 text-xs"
          :style="{
            background: sub.confirmed ? 'var(--mnt-status-ok-bg)' : 'var(--mnt-status-warn-bg)',
            color: sub.confirmed ? 'var(--mnt-status-ok)' : 'var(--mnt-status-warn)',
          }"
        >
          {{ sub.confirmed ? 'confirmed' : 'pending' }}
        </span>
      </div>
    </div>
  </div>
</template>
