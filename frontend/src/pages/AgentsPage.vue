<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { computed } from 'vue'

import { useEdition } from '@/composables/useEdition'
import FeatureGate from '@/components/FeatureGate.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import AgentsPanel from '@/commercial/components/agents/AgentsPanel.vue'
import { docUrl } from '@/utils/docs'
import { MonitorDot, Server, Boxes, ShieldCheck } from 'lucide-vue-next'
import UnlockCta from '@/components/UnlockCta.vue'

const { tierLimit } = useEdition()
const personalHostLimit = computed(() => tierLimit('personal', 'agent_hosts'))
const proHostLimit = computed(() => tierLimit('pro', 'agent_hosts'))
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">

      <!-- Header -->
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-mnt-primary">Agents</h1>
          <p class="mt-1 text-sm text-mnt-muted">
            Remote monitoring agents enrolled on this server
          </p>
        </div>
      </div>

      <FeatureHint
        storage-key="agents"
        title="Monitor remote hosts from a single server"
        :doc-href="docUrl('features/multihost/#agent-enrollment')"
      >
        Enroll lightweight agents on remote machines to stream their Docker, Swarm and Kubernetes state back to this server. Generate an enrollment token, run the install command on the host, and the agent appears below.
      </FeatureHint>

      <!-- Pro gate -->
      <FeatureGate feature="multihost">
        <AgentsPanel />

        <!-- Placeholder slot (Community Edition) -->
        <template #placeholder>
          <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-mnt-green-500/10 border border-mnt-green-500/20 flex items-center justify-center mb-4">
                <MonitorDot :size="22" class="text-mnt-green-400" />
              </div>
              <h2 class="text-base font-bold text-mnt-primary mb-1">Multi-host Agents</h2>
              <p class="text-sm text-mnt-muted max-w-md mb-6 leading-relaxed">
                Enroll lightweight agents on remote hosts to monitor Docker, Swarm and Kubernetes from this single server. No extra dashboards, no per-host setup.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li v-if="personalHostLimit !== null" class="flex items-start gap-3">
                  <Server :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Monitor up to {{ personalHostLimit }} remote hosts from one server with token-based enrollment,
                    or {{ proHostLimit === -1 ? 'an unlimited number' : `up to ${proHostLimit}` }} on Pro
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <Boxes :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Auto-detect Docker, Swarm and Kubernetes runtimes on each agent
                  </span>
                </li>
                <li class="flex items-start gap-3">
                  <ShieldCheck :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">
                    Secure, expiring enrollment tokens you can revoke at any time
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
