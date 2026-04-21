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
import { ref, onMounted, onUnmounted } from 'vue'
import { useAlertsStore } from '@/stores/alerts'
import ActiveAlerts from '@/components/ActiveAlerts.vue'
import AlertList from '@/components/AlertList.vue'
import ChannelManager from '@/components/ChannelManager.vue'
import SilenceRuleManager from '@/components/SilenceRuleManager.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'

const store = useAlertsStore()
const activeTab = ref<'history' | 'channels' | 'silence'>('history')

onMounted(() => {
  store.fetchAlerts()
  store.fetchActiveAlerts()
  store.fetchChannels()
  store.fetchSilenceRules()
  store.connectSSE()
  store.clearNewAlertCount()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-pb-primary">Alerts</h1>
      <p class="mt-1 text-sm text-slate-500">
        Alert history, notification channels, and silence rules
      </p>
    </div>

    <!-- Active alerts -->
    <div class="mb-6">
      <h2 class="mb-2 text-sm font-medium" style="color: var(--pb-text-secondary)">Active Alerts</h2>
      <ActiveAlerts />
    </div>

    <!-- Channel health summary -->
    <div v-if="store.channels.length > 0" class="mb-6 flex flex-wrap gap-2">
      <div
        v-for="ch in store.channels"
        :key="ch.id"
        class="flex items-center gap-1.5 rounded-full px-3 py-1 text-xs"
        :style="{
          border: ch.health === 'healthy'
            ? '1px solid var(--pb-status-ok)'
            : '1px solid var(--pb-status-down)',
          backgroundColor: ch.health === 'healthy'
            ? 'var(--pb-status-ok-bg)'
            : 'var(--pb-status-down-bg)',
          color: ch.health === 'healthy'
            ? 'var(--pb-status-ok)'
            : 'var(--pb-status-down)',
        }"
      >
        <span
          class="h-1.5 w-1.5 rounded-full"
          :style="{
            backgroundColor: ch.health === 'healthy'
              ? 'var(--pb-status-ok)'
              : 'var(--pb-status-down)',
          }"
        ></span>
        {{ ch.name }}
      </div>
    </div>

    <!-- Tab navigation -->
    <div class="mb-4" style="border-bottom: 1px solid var(--pb-border-default)">
      <nav class="-mb-px flex gap-6">
        <button
          @click="activeTab = 'history'"
          class="pb-2 text-sm font-medium min-h-[44px]"
          :style="{
            borderBottom: activeTab === 'history' ? '2px solid var(--pb-accent)' : '2px solid transparent',
            color: activeTab === 'history' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
          }"
        >
          History
        </button>
        <button
          @click="activeTab = 'channels'"
          class="pb-2 text-sm font-medium min-h-[44px]"
          :style="{
            borderBottom: activeTab === 'channels' ? '2px solid var(--pb-accent)' : '2px solid transparent',
            color: activeTab === 'channels' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
          }"
        >
          Channels
          <span
            v-if="store.channels.length"
            class="ml-1 rounded-full px-1.5 py-0.5 text-xs"
            style="background-color: var(--pb-bg-elevated); color: var(--pb-text-muted)"
          >
            {{ store.channels.length }}
          </span>
        </button>
        <button
          @click="activeTab = 'silence'"
          class="pb-2 text-sm font-medium min-h-[44px]"
          :style="{
            borderBottom: activeTab === 'silence' ? '2px solid var(--pb-accent)' : '2px solid transparent',
            color: activeTab === 'silence' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
          }"
        >
          Silence Rules
          <span
            v-if="store.activeSilenceCount"
            class="ml-1 rounded-full px-1.5 py-0.5 text-xs"
            style="background-color: var(--pb-status-warn-bg); color: var(--pb-status-warn)"
          >
            {{ store.activeSilenceCount }}
          </span>
        </button>
      </nav>
    </div>

    <!-- Tab content -->
    <AlertList v-if="activeTab === 'history'" />
    <template v-else-if="activeTab === 'channels'">
      <FeatureHint
        storage-key="alerts-channels"
        title="What are notification channels?"
        :doc-href="docUrl('features/alerts/#notification-channels')"
      >
        Channels deliver alerts to where your team is: Discord and generic HTTP webhooks are built-in; Slack, Microsoft Teams and Email (SMTP) are available on Pro. Each channel formats the payload natively, and you can send a test alert to verify delivery.
      </FeatureHint>
      <ChannelManager />
    </template>
    <template v-else-if="activeTab === 'silence'">
      <FeatureHint
        storage-key="alerts-silence"
        title="What are silence rules?"
        :doc-href="docUrl('features/alerts/#silence-rules')"
      >
        Silence rules suppress alert delivery during planned maintenance windows without discarding the events &mdash; the history still records everything. Match by source (endpoint, container, certificate&hellip;) and optionally by entity, set a time window, and alerts stop paging until it expires.
      </FeatureHint>
      <SilenceRuleManager />
    </template>
  </div>
  </div>
</template>
