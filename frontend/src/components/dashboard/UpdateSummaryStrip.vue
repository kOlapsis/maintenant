<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useUpdatesStore } from '@/stores/updates'
import { timeAgoFr } from '@/utils/time'
import ProFeatureGate from '@/components/ProFeatureGate.vue'
import { RefreshCw, AlertTriangle, ArrowUpCircle, CheckCircle, Shield } from 'lucide-vue-next'

const updates = useUpdatesStore()

onMounted(() => {
  updates.fetchSummary()
  updates.connectSSE()
})

onUnmounted(() => {
  updates.disconnectSSE()
})

const formatTime = timeAgoFr
</script>

<template>
  <div class="bg-[#151923] rounded-2xl border border-slate-800 p-5">
    <div class="flex items-center justify-between mb-4">
      <div class="flex items-center gap-2.5">
        <ArrowUpCircle :size="15" class="text-blue-500" />
        <h3 class="text-sm font-bold text-white">Mises à Jour</h3>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="updates.summary" class="text-[10px] text-slate-500 font-bold">
          Dernier scan : {{ formatTime(updates.summary.last_scan) }}
        </span>
        <button
          @click="updates.startScan()"
          :disabled="updates.scanning"
          class="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white rounded-lg text-xs font-bold transition-all flex items-center gap-1.5 shadow-lg shadow-blue-500/20"
        >
          <RefreshCw :size="11" :class="{ 'animate-spin': updates.scanning }" />
          {{ updates.scanning ? 'Scan...' : 'Vérifier' }}
        </button>
      </div>
    </div>

    <div v-if="updates.summary?.counts" class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <!-- Critical -->
      <RouterLink :to="{ name: 'updates' }" class="bg-[#0f1115] rounded-xl p-3 border border-slate-800 hover:border-slate-700 transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <AlertTriangle :size="11" class="text-rose-500" />
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Critiques</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.critical > 0 ? 'text-rose-400' : 'text-slate-600'">
          {{ updates.summary.counts.critical }}
        </p>
      </RouterLink>

      <!-- Recommended -->
      <RouterLink :to="{ name: 'updates' }" class="bg-[#0f1115] rounded-xl p-3 border border-slate-800 hover:border-slate-700 transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <ArrowUpCircle :size="11" class="text-amber-500" />
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Recommandées</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.recommended > 0 ? 'text-amber-400' : 'text-slate-600'">
          {{ updates.summary.counts.recommended }}
        </p>
      </RouterLink>

      <!-- Available -->
      <RouterLink :to="{ name: 'updates' }" class="bg-[#0f1115] rounded-xl p-3 border border-slate-800 hover:border-slate-700 transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <ArrowUpCircle :size="11" class="text-blue-500" />
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Disponibles</span>
        </div>
        <p class="text-xl font-black" :class="updates.summary.counts.available > 0 ? 'text-blue-400' : 'text-slate-600'">
          {{ updates.summary.counts.available }}
        </p>
      </RouterLink>

      <!-- Up to date -->
      <RouterLink :to="{ name: 'updates' }" class="bg-[#0f1115] rounded-xl p-3 border border-slate-800 hover:border-slate-700 transition-colors">
        <div class="flex items-center gap-1.5 mb-1">
          <CheckCircle :size="11" class="text-emerald-500" />
          <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">À jour</span>
        </div>
        <p class="text-xl font-black text-emerald-400">
          {{ updates.summary.counts.up_to_date }}
        </p>
      </RouterLink>
    </div>
  </div>
</template>
