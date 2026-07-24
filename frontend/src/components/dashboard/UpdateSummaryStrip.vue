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
import { onMounted, onUnmounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useUpdatesStore } from '@/stores/updates'
import { timeAgo } from '@/utils/time'
import { RefreshCw, AlertTriangle, ArrowUpCircle, CheckCircle } from 'lucide-vue-next'

const updates = useUpdatesStore()

onMounted(() => {
  updates.fetchSummary()
  updates.connectSSE()
})

onUnmounted(() => {
  updates.disconnectSSE()
})

const formatTime = timeAgo
</script>

<template>
  <div class="bg-mnt-surface rounded-xl sm:rounded-2xl border border-mnt-default p-3 sm:p-5">
    <div class="flex items-center justify-between mb-3 sm:mb-4">
      <div class="flex items-center gap-2.5">
        <ArrowUpCircle :size="15" class="text-mnt-green-500" />
        <h3 class="text-sm font-bold text-mnt-primary">Updates</h3>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="updates.summary" class="text-[10px] text-mnt-muted font-bold">
          Last scan: {{ formatTime(updates.summary.last_scan) }}
        </span>
        <button
          @click="updates.startScan()"
          :disabled="updates.scanning"
          class="px-3 py-1.5 bg-mnt-green-600 hover:bg-mnt-green-500 disabled:bg-mnt-elevated disabled:text-mnt-muted text-mnt-inverted rounded-lg text-xs font-bold transition-all flex items-center gap-1.5 shadow-lg shadow-mnt-green-500/20"
        >
          <RefreshCw :size="11" :class="{ 'animate-spin': updates.scanning }" />
          {{ updates.scanning ? 'Scan...' : 'Check' }}
        </button>
      </div>
    </div>

    <div v-if="updates.summary?.counts" class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <!-- Critical -->
      <RouterLink :to="{ name: 'updates' }" class="bg-mnt-primary rounded-xl p-3 border border-mnt-default hover:border-mnt-default transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <AlertTriangle :size="11" class="text-mnt-status-down" />
          <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Critical</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.critical > 0 ? 'text-mnt-status-down' : 'text-mnt-muted'">
          {{ updates.summary.counts.critical }}
        </p>
      </RouterLink>

      <!-- Recommended -->
      <RouterLink :to="{ name: 'updates' }" class="bg-mnt-primary rounded-xl p-3 border border-mnt-default hover:border-mnt-default transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <ArrowUpCircle :size="11" class="text-mnt-status-warn" />
          <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Recommended</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.recommended > 0 ? 'text-mnt-status-warn' : 'text-mnt-muted'">
          {{ updates.summary.counts.recommended }}
        </p>
      </RouterLink>

      <!-- Available -->
      <RouterLink :to="{ name: 'updates' }" class="bg-mnt-primary rounded-xl p-3 border border-mnt-default hover:border-mnt-default transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <ArrowUpCircle :size="11" class="text-mnt-green-500" />
          <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Available</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.available > 0 ? 'text-mnt-green-400' : 'text-mnt-muted'">
          {{ updates.summary.counts.available }}
        </p>
      </RouterLink>

      <!-- Up to date -->
      <RouterLink :to="{ name: 'updates' }" class="bg-mnt-primary rounded-xl p-3 border border-mnt-default hover:border-mnt-default transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <CheckCircle :size="11" class="text-mnt-status-ok" />
          <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Up to date</span>
        </div>
        <p class="text-xl font-black text-mnt-status-ok">
          {{ updates.summary.counts.up_to_date }}
        </p>
      </RouterLink>
    </div>
  </div>
</template>
