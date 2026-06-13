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
import { onMounted, onUnmounted, computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUpdatesStore } from '@/stores/updates'
import { useEdition } from '@/composables/useEdition'
import { timeAgo } from '@/utils/time'
import UpdateBadge from '@/components/UpdateBadge.vue'
import UpdateDetailPanel from '@/components/UpdateDetailPanel.vue'
import SlideOverPanel from '@/components/ui/SlideOverPanel.vue'
import type { ImageUpdate } from '@/services/updateApi'
import FeatureGate from '@/components/FeatureGate.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'
import {
  RefreshCw,
  AlertTriangle,
  ArrowUpCircle,
  CheckCircle,
  Shield,
  ChevronRight,
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
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
    case 'major': return 'text-pb-status-down'
    case 'minor': return 'text-pb-status-warn'
    case 'patch': return 'text-pb-green-400'
    default: return 'text-pb-muted'
  }
}

function shortTag(tag: string): string {
  if (/^(sha-|sha256:)?[0-9a-f]{12,}$/i.test(tag)) return tag.slice(0, 12)
  return tag
}

const formatTime = timeAgo

function openFromQuery() {
  const containerName = route.query.container as string | undefined
  if (!containerName || updates.updates.length === 0) return
  const match = updates.updates.find(u => u.container_name === containerName)
  if (match) {
    selectUpdate(match)
    router.replace({ query: {} })
  }
}

watch(() => updates.updates, openFromQuery)

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
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-pb-primary">Updates</h1>
          <p class="mt-1 text-sm text-pb-muted">
            Automatic container update detection
          </p>
        </div>
        <div class="flex items-center gap-3">
          <span v-if="updates.summary" class="text-[10px] text-pb-muted font-bold">
            Last scan: {{ formatTime(updates.summary.last_scan) }}
          </span>
          <button
            @click="updates.startScan()"
            :disabled="updates.scanning"
            class="px-4 py-2 bg-pb-green-600 hover:bg-pb-green-500 disabled:bg-pb-elevated disabled:text-pb-muted text-pb-inverted rounded-lg text-xs font-bold transition-all flex items-center gap-2 shadow-lg shadow-pb-green-500/20"
          >
            <RefreshCw :size="13" :class="{ 'animate-spin': updates.scanning }" />
            {{ updates.scanning ? 'Scanning...' : 'Check now' }}
          </button>
        </div>
      </div>

      <FeatureHint
        storage-key="updates"
        title="OCI digest scanning, digest-only or semver"
        :doc-href="docUrl('features/updates/#tag-filtering')"
      >
        maintenant scans the source registry of each image and compares digests. For semver tags, it compares versions while respecting variant suffixes (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">-alpine</code>, <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">-bookworm</code>). Constrain candidates with
        <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.update.tag-include</code>
        /
        <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">tag-exclude</code>
        (Go regex), pin a container's current version, or exclude whole images entirely.
      </FeatureHint>

      <!-- Summary Cards -->
      <div v-if="updates.summary?.counts" class="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <div class="bg-pb-surface rounded-xl p-4 border border-pb-default">
          <div class="flex items-center gap-1.5 mb-1">
            <AlertTriangle :size="11" class="text-pb-status-down" />
            <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Critical</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.critical > 0 ? 'text-pb-status-down' : 'text-pb-muted'">
            {{ updates.summary.counts.critical }}
          </p>
        </div>
        <div class="bg-pb-surface rounded-xl p-4 border border-pb-default">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-pb-status-warn" />
            <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Recommended</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.recommended > 0 ? 'text-pb-status-warn' : 'text-pb-muted'">
            {{ updates.summary.counts.recommended }}
          </p>
        </div>
        <div class="bg-pb-surface rounded-xl p-4 border border-pb-default">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-pb-green-500" />
            <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Available</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.available > 0 ? 'text-pb-green-400' : 'text-pb-muted'">
            {{ updates.summary.counts.available }}
          </p>
        </div>
        <div class="bg-pb-surface rounded-xl p-4 border border-pb-default">
          <div class="flex items-center gap-1.5 mb-1">
            <CheckCircle :size="11" class="text-pb-status-ok" />
            <span class="text-[10px] text-pb-muted font-bold uppercase tracking-widest">Up to date</span>
          </div>
          <p class="text-2xl font-black text-pb-status-ok">
            {{ updates.summary.counts.up_to_date }}
          </p>
        </div>
      </div>

      <!-- CVE summary (Pro) -->
      <div v-if="enabledCVE && updates.summary?.cve_counts && (updates.summary.cve_counts.critical > 0 || updates.summary.cve_counts.high > 0)" class="flex items-center gap-2 text-xs bg-pb-surface rounded-xl px-4 py-3 border border-pb-default">
        <Shield :size="13" class="text-pb-status-down" />
        <span class="text-pb-muted font-bold">Active CVEs:</span>
        <span v-if="updates.summary.cve_counts.critical > 0" class="text-pb-status-down font-bold">{{ updates.summary.cve_counts.critical }} critical</span>
        <span v-if="updates.summary.cve_counts.high > 0" class="text-pb-status-warn font-bold">{{ updates.summary.cve_counts.high }} high</span>
      </div>

      <!-- Update Groups -->
      <template v-for="(group, key) in {
        'Critical': groupedUpdates.critical,
        'Recommended': groupedUpdates.recommended,
        'Available': groupedUpdates.available,
        'Pinned': groupedUpdates.pinned,
      }" :key="key">
        <div v-if="group.length > 0" class="bg-pb-surface rounded-2xl border border-pb-default overflow-hidden">
          <div class="px-5 py-3 border-b border-pb-default flex items-center gap-2">
            <AlertTriangle v-if="key === 'Critical'" :size="13" class="text-pb-status-down" />
            <ArrowUpCircle v-else-if="key === 'Recommended'" :size="13" class="text-pb-status-warn" />
            <ArrowUpCircle v-else-if="key === 'Available'" :size="13" class="text-pb-green-500" />
            <Shield v-else :size="13" class="text-pb-muted" />
            <h3 class="text-sm font-bold text-pb-primary">{{ key }}</h3>
            <span class="text-[10px] text-pb-muted font-bold ml-1">({{ group.length }})</span>
          </div>

          <div class="divide-y divide-slate-800/40">
            <div
              v-for="u in group"
              :key="u.id"
              class="flex items-center gap-4 px-5 py-3 hover:bg-pb-elevated transition-all cursor-pointer group"
              @click="selectUpdate(u)"
            >
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-semibold text-pb-primary group-hover:text-pb-green-400 transition-colors truncate">
                    {{ u.container_name }}
                  </p>
                  <UpdateBadge :update="u" />
                </div>
                <p class="text-[10px] text-pb-muted mt-0.5 truncate">{{ u.image.split('@')[0] }}</p>
                <p v-if="u.status === 'pinned' && u.pin_reason" class="text-[10px] text-pb-muted mt-0.5 truncate italic">{{ u.pin_reason }}</p>
              </div>
              <div class="text-right shrink-0">
                <p :class="['text-xs font-bold', updateTypeColor(u.update_type)]">
                  {{ shortTag(u.current_tag) }} → {{ shortTag(u.latest_tag) }}
                </p>
                <p class="text-[10px] text-pb-muted mt-0.5">{{ formatTime(u.detected_at) }}</p>
              </div>
              <FeatureGate feature="risk_scoring">
                <div v-if="u.risk_score > 0" class="shrink-0 w-10 text-center">
                  <span
                    class="text-xs font-black"
                    :class="u.risk_score >= 81 ? 'text-pb-status-down' : u.risk_score >= 31 ? 'text-pb-status-warn' : 'text-pb-green-400'"
                  >{{ u.risk_score }}</span>
                </div>
              </FeatureGate>
              <ChevronRight :size="14" class="text-pb-muted group-hover:text-pb-muted shrink-0 transition-colors" />
            </div>
          </div>
        </div>
      </template>

      <!-- Empty state -->
      <div v-if="updates.updates.length === 0 && !updates.loading" class="flex flex-col items-center justify-center py-16">
        <CheckCircle :size="40" class="text-emerald-500/30 mb-3" />
        <p class="text-sm text-pb-muted font-medium">All containers are up to date</p>
        <p class="text-[10px] text-pb-muted mt-1">Run a scan to check for available updates</p>
      </div>
    </div>

    <!-- Detail Panel -->
    <SlideOverPanel v-model:open="slideOpen" :title="selectedUpdate?.container_name || ''">
      <UpdateDetailPanel v-if="selectedUpdate" :container-id="selectedUpdate.container_id" />
    </SlideOverPanel>
  </div>
</template>
