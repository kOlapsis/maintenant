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
import { onMounted, onUnmounted, computed, nextTick, ref, watch } from 'vue'
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
import SectionHeader from '@/components/ui/SectionHeader.vue'
import StatusDot from '@/components/ui/StatusDot.vue'
import OsSupportTimeline from '@/components/OsSupportTimeline.vue'
import {
  groupHostsByOS,
  osAtRisk,
  osAtRiskTone,
  osStateTextClass,
  osSupportLabel,
  osSupportSeverity,
  osTimeLeft,
  osTotal,
} from '@/utils/osSupport'
import { docUrl } from '@/utils/docs'
import {
  RefreshCw,
  AlertTriangle,
  ArrowUpCircle,
  CheckCircle,
  Shield,
  Server,
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

const osCounts = computed(() => updates.summary?.os_counts)

const osGroups = computed(() => groupHostsByOS(updates.hosts))

const MAX_HOST_CHIPS = 6

function scrollToOS() {
  document.getElementById('os')?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const eolTableNote = computed(() => {
  const table = updates.eolTable
  if (!table) return ''
  const day = table.fetched_at ? new Date(table.fetched_at).toLocaleDateString() : 'unknown date'
  return table.source === 'endoflife.date'
    ? `Dates from endoflife.date, refreshed ${day}`
    : `Embedded support table (${day})`
})

function hostName(host: { label: string; hostname: string }): string {
  return host.label || host.hostname
}

watch(
  () => route.hash === '#os' && updates.hosts.length > 0,
  async (ready) => {
    if (!ready) return
    await nextTick()
    scrollToOS()
  },
  { immediate: true },
)

function updateTypeColor(type_: string): string {
  switch (type_) {
    case 'major': return 'text-mnt-status-down'
    case 'minor': return 'text-mnt-status-warn'
    case 'patch': return 'text-mnt-green-400'
    default: return 'text-mnt-muted'
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
  updates.fetchHostOS()
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
          <h1 class="text-2xl font-black text-mnt-primary">Updates</h1>
          <p class="mt-1 text-sm text-mnt-muted">
            Automatic container update detection
          </p>
        </div>
        <div class="flex items-center gap-3">
          <span v-if="updates.summary" class="text-[10px] text-mnt-muted font-bold">
            Last scan: {{ formatTime(updates.summary.last_scan) }}
          </span>
          <button
            @click="updates.startScan()"
            :disabled="updates.scanning"
            class="px-4 py-2 bg-mnt-green-600 hover:bg-mnt-green-500 disabled:bg-mnt-elevated disabled:text-mnt-muted text-mnt-inverted rounded-lg text-xs font-bold transition-all flex items-center gap-2 shadow-lg shadow-mnt-green-500/20"
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
        maintenant scans the source registry of each image and compares digests. For semver tags, it compares versions while respecting variant suffixes (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">-alpine</code>, <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">-bookworm</code>). Constrain candidates with
        <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">maintenant.update.tag-include</code>
        /
        <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">tag-exclude</code>
        (Go regex), pin a container's current version, or exclude whole images entirely.
      </FeatureHint>

      <!-- Summary Cards -->
      <div v-if="updates.summary?.counts" class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
        <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default">
          <div class="flex items-center gap-1.5 mb-1">
            <AlertTriangle :size="11" class="text-mnt-status-down" />
            <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Critical</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.critical > 0 ? 'text-mnt-status-down' : 'text-mnt-muted'">
            {{ updates.summary.counts.critical }}
          </p>
        </div>
        <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-mnt-status-warn" />
            <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Recommended</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.recommended > 0 ? 'text-mnt-status-warn' : 'text-mnt-muted'">
            {{ updates.summary.counts.recommended }}
          </p>
        </div>
        <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default">
          <div class="flex items-center gap-1.5 mb-1">
            <ArrowUpCircle :size="11" class="text-mnt-green-500" />
            <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Available</span>
          </div>
          <p class="text-2xl font-black" :class="updates.summary.counts.available > 0 ? 'text-mnt-green-400' : 'text-mnt-muted'">
            {{ updates.summary.counts.available }}
          </p>
        </div>
        <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default">
          <div class="flex items-center gap-1.5 mb-1">
            <CheckCircle :size="11" class="text-mnt-status-ok" />
            <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">Up to date</span>
          </div>
          <p class="text-2xl font-black text-mnt-status-ok">
            {{ updates.summary.counts.up_to_date }}
          </p>
        </div>
        <a
          v-if="osCounts"
          href="#os"
          class="bg-mnt-surface rounded-xl p-4 border border-mnt-default hover:bg-mnt-elevated transition-colors"
          @click.prevent="scrollToOS"
        >
          <div class="flex items-center gap-1.5 mb-1">
            <Server :size="11" :class="osAtRiskTone(osCounts)" />
            <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">OS at risk</span>
          </div>
          <p class="text-2xl font-black" :class="osAtRiskTone(osCounts)">
            {{ osAtRisk(osCounts) }}<span class="ml-1 text-xs font-bold text-mnt-muted">/ {{ osTotal(osCounts) }} {{ osTotal(osCounts) === 1 ? 'host' : 'hosts' }}</span>
          </p>
        </a>
      </div>

      <!-- Operating systems -->
      <section v-if="osGroups.length > 0" id="os" class="scroll-mt-4 bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
        <div class="px-4 sm:px-5 py-3 border-b border-mnt-default">
          <SectionHeader title="Operating systems" :icon="Server" :count="updates.hosts.length" />
        </div>

        <ul>
          <li
            v-for="group in osGroups"
            :key="group.key"
            class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5 px-4 sm:px-5 py-2.5 border-t border-mnt-subtle first:border-t-0 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_12rem_9rem]"
          >
            <div class="flex min-w-0 items-center gap-2.5">
              <StatusDot :severity="osSupportSeverity(group.state)" :label="osSupportLabel(group.state)" />
              <p
                class="truncate text-sm font-bold text-mnt-primary"
                :title="group.prettyName || group.name"
              >
                <template v-if="group.state === 'unknown'">
                  {{ group.hosts.length }} {{ group.hosts.length === 1 ? 'host' : 'hosts' }} without OS
                </template>
                <template v-else>{{ group.name }}</template>
              </p>
            </div>

            <p
              v-if="group.state !== 'unknown' && group.state !== 'untracked'"
              class="text-right text-sm font-black sm:order-last"
              :class="osStateTextClass(group.state)"
            >{{ osTimeLeft(group.support) }}</p>

            <ul class="col-span-2 flex min-w-0 flex-wrap gap-1 sm:col-span-1">
              <li
                v-for="host in group.hosts.slice(0, MAX_HOST_CHIPS)"
                :key="host.agent_id"
                class="max-w-[12rem] truncate rounded-md bg-mnt-elevated px-1.5 py-0.5 text-[10px] font-semibold text-mnt-secondary"
                :title="host.hostname"
              >{{ hostName(host) }}</li>
              <li
                v-if="group.hosts.length > MAX_HOST_CHIPS"
                class="rounded-md px-1.5 py-0.5 text-[10px] font-semibold text-mnt-muted"
                :title="group.hosts.slice(MAX_HOST_CHIPS).map(hostName).join(', ')"
              >+{{ group.hosts.length - MAX_HOST_CHIPS }}</li>
            </ul>

            <p v-if="group.state === 'unknown'" class="col-span-2 truncate text-xs text-mnt-secondary" :title="group.hints.join(' · ')">
              {{ group.hints.join(' · ') }}
            </p>
            <p v-else-if="group.state === 'untracked'" class="col-span-2 text-xs text-mnt-muted sm:text-right">
              No published end-of-life dates
            </p>
            <OsSupportTimeline v-else class="col-span-2 sm:col-span-1" :support="group.support" compact />
          </li>
        </ul>

        <p v-if="eolTableNote" class="px-4 sm:px-5 py-3 border-t border-mnt-subtle text-[10px] text-mnt-muted">{{ eolTableNote }}</p>
      </section>

      <!-- CVE summary (Pro) -->
      <div v-if="enabledCVE && updates.summary?.cve_counts && (updates.summary.cve_counts.critical > 0 || updates.summary.cve_counts.high > 0)" class="flex items-center gap-2 text-xs bg-mnt-surface rounded-xl px-4 py-3 border border-mnt-default">
        <Shield :size="13" class="text-mnt-status-down" />
        <span class="text-mnt-muted font-bold">Active CVEs:</span>
        <span v-if="updates.summary.cve_counts.critical > 0" class="text-mnt-status-down font-bold">{{ updates.summary.cve_counts.critical }} critical</span>
        <span v-if="updates.summary.cve_counts.high > 0" class="text-mnt-status-warn font-bold">{{ updates.summary.cve_counts.high }} high</span>
      </div>

      <!-- Update Groups -->
      <template v-for="(group, key) in {
        'Critical': groupedUpdates.critical,
        'Recommended': groupedUpdates.recommended,
        'Available': groupedUpdates.available,
        'Pinned': groupedUpdates.pinned,
      }" :key="key">
        <div v-if="group.length > 0" class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
          <div class="px-5 py-3 border-b border-mnt-default flex items-center gap-2">
            <AlertTriangle v-if="key === 'Critical'" :size="13" class="text-mnt-status-down" />
            <ArrowUpCircle v-else-if="key === 'Recommended'" :size="13" class="text-mnt-status-warn" />
            <ArrowUpCircle v-else-if="key === 'Available'" :size="13" class="text-mnt-green-500" />
            <Shield v-else :size="13" class="text-mnt-muted" />
            <h3 class="text-sm font-bold text-mnt-primary">{{ key }}</h3>
            <span class="text-[10px] text-mnt-muted font-bold ml-1">({{ group.length }})</span>
          </div>

          <div class="divide-y divide-slate-800/40">
            <div
              v-for="u in group"
              :key="u.id"
              class="flex items-center gap-4 px-5 py-3 hover:bg-mnt-elevated transition-all cursor-pointer group"
              @click="selectUpdate(u)"
            >
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-semibold text-mnt-primary group-hover:text-mnt-green-400 transition-colors truncate">
                    {{ u.container_name }}
                  </p>
                  <UpdateBadge :update="u" />
                </div>
                <p class="text-[10px] text-mnt-muted mt-0.5 truncate">{{ u.image.split('@')[0] }}</p>
                <p v-if="u.status === 'pinned' && u.pin_reason" class="text-[10px] text-mnt-muted mt-0.5 truncate italic">{{ u.pin_reason }}</p>
              </div>
              <div class="text-right shrink-0">
                <p :class="['text-xs font-bold', updateTypeColor(u.update_type)]">
                  {{ shortTag(u.current_tag) }} → {{ shortTag(u.latest_tag) }}
                </p>
                <p class="text-[10px] text-mnt-muted mt-0.5">{{ formatTime(u.detected_at) }}</p>
              </div>
              <FeatureGate feature="risk_scoring">
                <div v-if="u.risk_score > 0" class="shrink-0 w-10 text-center">
                  <span
                    class="text-xs font-black"
                    :class="u.risk_score >= 81 ? 'text-mnt-status-down' : u.risk_score >= 31 ? 'text-mnt-status-warn' : 'text-mnt-green-400'"
                  >{{ u.risk_score }}</span>
                </div>
              </FeatureGate>
              <ChevronRight :size="14" class="text-mnt-muted group-hover:text-mnt-muted shrink-0 transition-colors" />
            </div>
          </div>
        </div>
      </template>

      <!-- Empty state -->
      <div v-if="updates.updates.length === 0 && !updates.loading" class="flex flex-col items-center justify-center py-16">
        <CheckCircle :size="40" class="text-emerald-500/30 mb-3" />
        <p class="text-sm text-mnt-muted font-medium">All containers are up to date</p>
        <p class="text-[10px] text-mnt-muted mt-1">Run a scan to check for available updates</p>
      </div>
    </div>

    <!-- Detail Panel -->
    <SlideOverPanel v-model:open="slideOpen" :title="selectedUpdate?.container_name || ''">
      <UpdateDetailPanel v-if="selectedUpdate" :container-id="selectedUpdate.container_id" />
    </SlideOverPanel>
  </div>
</template>
