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
import { computed } from 'vue'
import { usePostureStore } from '@/commercial/stores/posture'
import { useContainersStore } from '@/stores/containers'
import { timeAgo } from '@/utils/time'
import FeatureGate from '@/components/FeatureGate.vue'
import SecurityPosturePanel from '@/commercial/components/posture/SecurityPosturePanel.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'
import UnlockCta from '@/components/UnlockCta.vue'

import { ShieldCheck, AlertTriangle, CheckCircle2, BarChart3 } from 'lucide-vue-next'

const store = usePostureStore()
const containerStore = useContainersStore()

const posture = computed(() => store.posture)
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

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
        <SecurityPosturePanel />

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

              <UnlockCta />
            </div>
          </div>
        </template>

      </FeatureGate>

    </div>
  </div>
</template>
