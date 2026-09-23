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
import { onMounted } from 'vue'

import FeatureGate from '@/components/FeatureGate.vue'
import EscalationPanel from '@/commercial/components/escalation/EscalationPanel.vue'
import { useEscalationStore } from '@/commercial/stores/escalation'
import UnlockCta from '@/components/UnlockCta.vue'
import { BellRing, CheckCircle2, ShieldAlert } from 'lucide-vue-next'

const store = useEscalationStore()

onMounted(() => {
  store.fetchPolicies()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">

      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-mnt-primary">Escalation Policies</h1>
          <p class="mt-1 text-sm text-mnt-muted">
            Multi-level alert routing with automatic escalation chains
          </p>
        </div>
        <div v-if="store.limits" class="flex items-center gap-3">
          <span class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest">
            <template v-if="store.limits.max_active === -1">
              {{ store.limits.current_active }} active
            </template>
            <template v-else>
              {{ store.limits.current_active }}/{{ store.limits.max_active }} active
            </template>
          </span>
        </div>
      </div>

      <FeatureGate feature="alert_escalation">
        <EscalationPanel />

        <!-- Placeholder slot (Community Edition) -->
        <template #placeholder>
          <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-mnt-green-500/10 border border-mnt-green-500/20 flex items-center justify-center mb-4">
                <BellRing :size="22" class="text-mnt-green-400" />
              </div>
              <h2 class="text-base font-bold text-mnt-primary mb-1">Escalation policies</h2>
              <p class="text-sm text-mnt-muted max-w-md mb-6 leading-relaxed">
                Automatically escalate unacknowledged alerts through a chain of notification levels — each with a configurable delay and a distinct set of channels.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Multi-level chains: page different teams at escalating delays
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Acknowledgment or resolution automatically stops the escalation
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <ShieldAlert :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Full audit trail: every delivery attempt logged with status and error
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
