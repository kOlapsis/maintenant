<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref } from 'vue'
import { RadioTower } from 'lucide-vue-next'
import { useEdition } from '@/composables/useEdition'
import { docUrl } from '@/utils/docs'
import OutboundHeartbeatsPanel from '@/components/heartbeats/OutboundHeartbeatsPanel.vue'
import UiButton from '@/components/ui/UiButton.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import EmptyState from '@/components/ui/EmptyState.vue'

const { isDemo } = useEdition()

const showCreateForm = ref(false)
</script>

<template>
  <div class="p-3 sm:p-6">
    <div class="max-w-7xl mx-auto">
      <div class="mb-6 flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-black text-mnt-primary">Outbound heartbeats</h1>
          <p class="mt-1 text-sm" :style="{ color: 'var(--mnt-text-muted)' }">
            Ping other Maintenant instances so they alert if this one goes down
          </p>
        </div>
        <UiButton v-if="!isDemo" variant="primary" @click="showCreateForm = !showCreateForm">
          {{ showCreateForm ? 'Cancel' : 'New target' }}
        </UiButton>
      </div>

      <template v-if="!isDemo">
        <FeatureHint
          storage-key="outbound-heartbeats"
          title="Let another instance watch this one"
          :doc-href="docUrl('features/heartbeats/#outbound-heartbeats')"
        >
          Create a heartbeat monitor on another Maintenant instance, then paste its ping URL
          (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--mnt-bg-elevated); color: var(--mnt-text-secondary)">/ping/{uuid}</code>)
          as a target here. This instance pings it on the interval you choose &mdash; if this instance goes down, the other one raises the alert.
        </FeatureHint>

        <OutboundHeartbeatsPanel v-model:show-create-form="showCreateForm" />
      </template>

      <EmptyState
        v-else
        :icon="RadioTower"
        title="Unavailable in demo mode"
        description="Outbound heartbeats ping real URLs on other instances, which this read-only demo does not allow."
      />
    </div>
  </div>
</template>
