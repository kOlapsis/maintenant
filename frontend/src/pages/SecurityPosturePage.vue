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
import { inject, onMounted, onUnmounted, computed } from 'vue'
import { usePostureStore } from '@/stores/posture'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'
import { timeAgo } from '@/utils/time'
import PostureScoreBadge from '@/components/PostureScoreBadge.vue'
import PostureContainerList from '@/components/PostureContainerList.vue'
import FeatureGate from '@/components/FeatureGate.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'
import { detailSlideOverKey } from '@/composables/useDetailSlideOver'
import { RouterLink } from 'vue-router'
import { ShieldCheck, AlertTriangle, CheckCircle2, BarChart3 } from 'lucide-vue-next'

const { hasFeature } = useEdition()
const store = usePostureStore()
const containerStore = useContainersStore()
const { openDetail } = inject(detailSlideOverKey)!

const isAvailable = computed(() => hasFeature('security_posture'))
const posture = computed(() => store.posture)

function handleSelectContainer(containerId: string) {
  openDetail('container', containerId)
}

onMounted(() => {
  if (isAvailable.value) {
    store.fetchPosture()
    store.connectSSE()
    containerStore.fetchContainers()
  }
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 mnt-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-mnt-primary">Security Posture</h1>
          <p class="mt-1 text-sm text-mnt-muted">
            <template v-if="posture">{{ posture.scored_count }}/{{ posture.container_count }} containers scored</template>
            <template v-else>Infrastructure-wide security scoring</template>
          </p>
        </div>
        <div v-if="posture" class="flex items-center gap-3">
          <span v-if="posture.is_partial" class="flex items-center gap-1.5 text-[10px] text-mnt-status-warn font-bold">
            <AlertTriangle :size="11" />
            Partial data
          </span>
          <span class="text-[10px] text-mnt-muted font-bold">
            Updated {{ timeAgo(posture.computed_at) }}
          </span>
        </div>
      </div>

      <FeatureHint
        storage-key="posture"
        title="Scored view of your infrastructure risk"
        :doc-href="docUrl('features/security/#security-posture-dashboard')"
      >
        The posture score weights network exposure, configuration risks (privileged, host network), and pending updates across all {{ containerStore.runtimeLabel }} containers. Drill into individual containers to see the underlying insights, and <em>acknowledge</em> known findings to exclude them from the score with an audit trail.
      </FeatureHint>

      <!-- Pro gate -->
      <FeatureGate feature="security_posture">

        <!-- Loading -->
        <template v-if="store.loading && !posture">
          <div class="space-y-4">
            <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
              <div v-for="i in 6" :key="i" class="h-24 animate-pulse rounded-xl bg-mnt-elevated/50" />
            </div>
          </div>
        </template>

        <!-- Posture dashboard -->
        <template v-else-if="posture">
          <!-- Score + Category summary -->
          <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
            <!-- Global score card -->
            <div class="bg-mnt-surface rounded-xl p-4 border border-mnt-default flex flex-col items-center justify-center">
              <PostureScoreBadge :score="posture.score" :color="posture.color" size="md" />
              <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mt-2">Score</p>
            </div>

            <!-- Category cards -->
            <div
              v-for="cat in posture.categories"
              :key="cat.name"
              class="bg-mnt-surface rounded-xl p-4 border border-mnt-default"
            >
              <div class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">{{ cat.name.replace('_', ' ') }}</div>
              <p class="text-2xl font-black" :class="cat.total_issues > 0 ? 'text-mnt-status-warn' : 'text-mnt-muted'">
                {{ cat.total_issues }}
              </p>
              <p class="text-[10px] text-mnt-muted mt-0.5">{{ cat.summary }}</p>
            </div>
          </div>

          <!-- Top risks -->
          <div v-if="posture.top_risks.length > 0">
            <h2 class="text-sm font-bold text-mnt-primary mb-3">Top Risks</h2>
            <PostureContainerList :risks="posture.top_risks" @select="handleSelectContainer" />
          </div>
        </template>

        <!-- No data -->
        <div v-else class="flex flex-col items-center justify-center py-16">
          <ShieldCheck :size="40" class="text-mnt-muted mb-3" />
          <p class="text-sm text-mnt-muted font-medium">No posture data available</p>
          <p class="text-[10px] text-mnt-muted mt-1">Make sure containers are being monitored</p>
        </div>

        <!-- Placeholder slot (Community Edition) -->
        <template #placeholder>
          <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-mnt-green-500/10 border border-mnt-green-500/20 flex items-center justify-center mb-4">
                <ShieldCheck :size="22" class="text-mnt-green-400" />
              </div>
              <h2 class="text-base font-bold text-mnt-primary mb-1">Security Posture</h2>
              <p class="text-sm text-mnt-muted max-w-md mb-6 leading-relaxed">
                Get an infrastructure-wide security score that weights network exposure, configuration risks, and pending updates across every monitored container.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Single weighted score with per-category breakdown (network, config, updates)
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <BarChart3 :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Drill into the riskiest containers, sorted by impact on the global score
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <AlertTriangle :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Acknowledge known findings to exclude them from the score with full audit trail
                  </span>
                </li>
              </ul>

              <RouterLink
                to="/pro-edition"
                class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-semibold bg-mnt-green-600 hover:bg-mnt-green-500 text-mnt-inverted shadow-lg shadow-mnt-green-500/20 transition-colors"
              >
                Unlock with Pro
              </RouterLink>
            </div>
          </div>
        </template>

      </FeatureGate>

    </div>
  </div>
</template>
