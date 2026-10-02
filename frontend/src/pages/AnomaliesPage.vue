<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { Radar, CheckCircle2, TrendingUp, BellRing } from 'lucide-vue-next'
import FeatureGate from '@/components/FeatureGate.vue'
import UnlockCta from '@/components/UnlockCta.vue'
import AnomaliesPanel from '@/commercial/components/anomaly/AnomaliesPanel.vue'
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="max-w-7xl mx-auto space-y-6 pb-12">
      <div>
        <h1 class="text-2xl font-black text-mnt-primary">Anomalies</h1>
        <p class="mt-1 text-sm text-mnt-muted">Behavioral resource anomaly detection</p>
      </div>

      <FeatureGate feature="anomaly_detection">
        <AnomaliesPanel />

        <template #placeholder>
          <div class="bg-mnt-surface rounded-2xl border border-mnt-default overflow-hidden">
            <div class="px-6 py-10 flex flex-col items-center text-center">
              <div class="w-12 h-12 rounded-xl bg-mnt-green-500/10 border border-mnt-green-500/20 flex items-center justify-center mb-4">
                <Radar :size="22" class="text-mnt-green-400" />
              </div>
              <h2 class="text-base font-bold text-mnt-primary mb-1">Behavioral anomaly detection</h2>
              <p class="text-sm text-mnt-muted max-w-md mb-6 leading-relaxed">
                Learn what each container normally does at every hour of the week, then catch spikes, slow drift and level shifts without a threshold to tune.
              </p>

              <ul class="text-left space-y-3 mb-8 w-full max-w-sm">
                <li class="flex items-start gap-3">
                  <CheckCircle2 :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">Baselines learned per series and per hour of the week</span>
                </li>
                <li class="flex items-start gap-3">
                  <TrendingUp :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">Spikes, slow drift and level shifts, silent while a series is still learning</span>
                </li>
                <li class="flex items-start gap-3">
                  <BellRing :size="15" class="text-mnt-green-400 mt-0.5 shrink-0" />
                  <span class="text-sm text-mnt-secondary">Alerts through your existing channels, triggers and escalation policies</span>
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
