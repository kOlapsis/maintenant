<script setup lang="ts">
import { onMounted, onUnmounted, computed, ref } from 'vue'
import { useUpdatesStore } from '@/stores/updates'
import { useEdition } from '@/composables/useEdition'
import { timeAgo } from '@/utils/time'
import UpdateBadge from '@/components/UpdateBadge.vue'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import type { ImageUpdate } from '@/services/updateApi'
import FeatureGate from '@/components/FeatureGate.vue'
import {
  RefreshCw,
  AlertTriangle,
  ArrowUpCircle,
  CheckCircle,
  Shield,
  ChevronRight,
} from 'lucide-vue-next'

const { edition } = useEdition()
const updates = useUpdatesStore()

const selectedUpdate = ref<ImageUpdate | null>(null)
const slideOpen = ref(false)
const filterStatus = ref<string>('')

function selectUpdate(u: ImageUpdate) {
  selectedUpdate.value = u
  slideOpen.value = true
}

const groupedUpdates = computed(() => {
  const all = updates.updates.filter(u => {
    if (filterStatus.value && u.status !== filterStatus.value) return false
    return true
  })

  const pinned = all.filter(u => u.status === 'pinned')
  const nonPinned = all.filter(u => u.status !== 'pinned')
  const critical = nonPinned.filter(u => u.risk_score >= 81)
  const recommended = nonPinned.filter(u => u.risk_score >= 31 && u.risk_score < 81)
  const available = nonPinned.filter(u => u.risk_score < 31)

  return { critical, recommended, available, pinned }
})

const enabledCVE = computed(() => edition.value?.features['cve_enrichment'] === true)

function updateTypeColor(type_: string): string {
  switch (type_) {
    case 'major': return 'text-rose-400'
    case 'minor': return 'text-amber-400'
    case 'patch': return 'text-blue-400'
    default: return 'text-slate-400'
  }
}

const formatTime = timeAgo

onMounted(() => {
  updates.fetchAllUpdates()
  updates.fetchSummary()
  updates.connectSSE()
})

onUnmounted(() => {
  updates.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-bold text-white">Updates</h1>
          <p class="text-xs text-slate-500 mt-0.5">
            Automatic container update detection
          </p>
        </div>
        <div class="flex items-center gap-3">
          <span v-if="updates.summary" class="text-[10px] text-slate-500 font-bold">
            Last scan: {{ formatTime(updates.summary.last_scan) }}
          </span>
          <button
            @click="updates.startScan()"
            :disabled="updates.scanning"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-slate-700 disabled:text-slate-500 text-white rounded-lg text-xs font-bold transition-all flex items-center gap-2 shadow-lg shadow-blue-500/20"
          >
            <RefreshCw :size="13" :class="{ 'animate-spin': updates.scanning }" />
            {{ updates.scanning ? 'Scanning...' : 'Check now' }}
          </button>
        </div>
      </div>

      <!-- Summary Cards -->
      <div v-if="updates.summary?.counts" class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div class="bg-[#151923] rounded-xl p-4 border border-slate-800">
          <div class="flex items-center gap-1.5 mb-1">
            <AlertTriangle :size="11" class="text-rose-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Critical</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.critical > 0 ? 'text-rose-400' : 'text-slate-600'">
            {{ updates.summary.counts.critical }}
          </p>
        </div>
        <div class="bg-[#151923] rounded-xl p-4 border border-slate-800">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-amber-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Recommended</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.recommended > 0 ? 'text-amber-400' : 'text-slate-600'">
            {{ updates.summary.counts.recommended }}
          </p>
        </div>
        <div class="bg-[#151923] rounded-xl p-4 border border-slate-800">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-blue-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Available</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.available > 0 ? 'text-blue-400' : 'text-slate-600'">
            {{ updates.summary.counts.available }}
          </p>
        </div>
        <div class="bg-[#151923] rounded-xl p-4 border border-slate-800">
          <div class="flex items-center gap-1.5 mb-1">
            <CheckCircle :size="11" class="text-emerald-500" />
            <span class="text-[10px] text-slate-500 font-bold uppercase tracking-widest">Up to date</span>
          </div>
          <p class="text-2xl font-black text-emerald-400">
            {{ updates.summary.counts.up_to_date }}
          </p>
        </div>
      </div>

      <!-- CVE summary (Pro) -->
      <div v-if="enabledCVE && updates.summary?.cve_counts && (updates.summary.cve_counts.critical > 0 || updates.summary.cve_counts.high > 0)" class="flex items-center gap-2 text-xs bg-[#151923] rounded-xl px-4 py-3 border border-slate-800">
        <Shield :size="13" class="text-rose-500" />
        <span class="text-slate-400 font-bold">Active CVEs:</span>
        <span v-if="updates.summary.cve_counts.critical > 0" class="text-rose-400 font-bold">{{ updates.summary.cve_counts.critical }} critical</span>
        <span v-if="updates.summary.cve_counts.high > 0" class="text-amber-400 font-bold">{{ updates.summary.cve_counts.high }} high</span>
      </div>

      <!-- Update Groups -->
      <template v-for="(group, key) in {
        'Critical': groupedUpdates.critical,
        'Recommended': groupedUpdates.recommended,
        'Available': groupedUpdates.available,
        'Pinned': groupedUpdates.pinned,
      }" :key="key">
        <div v-if="group.length > 0" class="bg-[#151923] rounded-2xl border border-slate-800 overflow-hidden">
          <div class="px-5 py-3 border-b border-slate-800 flex items-center gap-2">
            <AlertTriangle v-if="key === 'Critical'" :size="13" class="text-rose-500" />
            <ArrowUpCircle v-else-if="key === 'Recommended'" :size="13" class="text-amber-500" />
            <ArrowUpCircle v-else-if="key === 'Available'" :size="13" class="text-blue-500" />
            <Shield v-else :size="13" class="text-slate-500" />
            <h3 class="text-sm font-bold text-white">{{ key }}</h3>
            <span class="text-[10px] text-slate-500 font-bold ml-1">({{ group.length }})</span>
          </div>

          <div class="divide-y divide-slate-800/40">
            <div
              v-for="u in group"
              :key="u.id"
              class="flex items-center gap-4 px-5 py-3 hover:bg-slate-800/25 transition-all cursor-pointer group"
              @click="selectUpdate(u)"
            >
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-semibold text-slate-100 group-hover:text-blue-400 transition-colors truncate">
                    {{ u.container_name }}
                  </p>
                  <UpdateBadge :update="u" />
                </div>
                <p class="text-[10px] text-slate-600 mt-0.5 truncate">{{ u.image }}</p>
              </div>
              <div class="text-right shrink-0">
                <p :class="['text-xs font-bold', updateTypeColor(u.update_type)]">
                  {{ u.current_tag }} → {{ u.latest_tag }}
                </p>
                <p class="text-[10px] text-slate-600 mt-0.5">{{ formatTime(u.detected_at) }}</p>
              </div>
              <FeatureGate feature="risk_scoring">
                <div v-if="u.risk_score > 0" class="shrink-0 w-10 text-center">
                  <span
                    class="text-xs font-black"
                    :class="u.risk_score >= 81 ? 'text-rose-400' : u.risk_score >= 31 ? 'text-amber-400' : 'text-blue-400'"
                  >{{ u.risk_score }}</span>
                </div>
              </FeatureGate>
              <ChevronRight :size="14" class="text-slate-700 group-hover:text-slate-400 shrink-0 transition-colors" />
            </div>
          </div>
        </div>
      </template>

      <!-- Empty state -->
      <div v-if="updates.updates.length === 0 && !updates.loading" class="flex flex-col items-center justify-center py-16">
        <CheckCircle :size="40" class="text-emerald-500/30 mb-3" />
        <p class="text-sm text-slate-600 font-medium">All containers are up to date</p>
        <p class="text-[10px] text-slate-700 mt-1">Run a scan to check for available updates</p>
      </div>
    </div>

    <!-- Detail Panel -->
    <SlideOverPanel v-model:open="slideOpen" :title="selectedUpdate?.container_name || ''">
      <UpdateDetailPanel v-if="selectedUpdate" :container-id="selectedUpdate.container_id" />
    </SlideOverPanel>
  </div>
</template>
