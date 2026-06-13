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
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAlertsStore } from '@/stores/alerts'
import { useTriggersStore } from '@/stores/triggers'
import ActiveAlerts from '@/components/ActiveAlerts.vue'
import AlertList from '@/components/AlertList.vue'
import TriggerManager from '@/components/TriggerManager.vue'
import SilenceRuleManager from '@/components/SilenceRuleManager.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'

type Tab = 'history' | 'triggers' | 'silence'

const route = useRoute()
const router = useRouter()
const store = useAlertsStore()
const triggersStore = useTriggersStore()

const activeTab = computed<Tab>({
  get: () => {
    const t = route.params.tab as string
    if (t === 'channels') return 'triggers' // legacy redirect
    if (t === 'triggers' || t === 'silence' || t === 'history') return t
    return 'history'
  },
  set: (tab: Tab) => router.replace({ name: 'alerts', params: { tab } }),
})

onMounted(() => {
  store.fetchAlerts()
  store.fetchActiveAlerts()
  store.fetchSilenceRules()
  triggersStore.fetchTriggers()
  store.connectSSE()
  triggersStore.connectSSE()
  store.clearNewAlertCount()
})

onUnmounted(() => {
  store.disconnectSSE()
  triggersStore.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
  <div class="max-w-7xl mx-auto">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-pb-primary">Alerts</h1>
      <p class="mt-1 text-sm text-pb-muted">
        Alert history, routing triggers, and silence rules
      </p>
    </div>

    <!-- Active alerts -->
    <div class="mb-6">
      <h2 class="mb-2 text-sm font-medium" style="color: var(--pb-text-secondary)">Active Alerts</h2>
      <ActiveAlerts />
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
          @click="activeTab = 'triggers'"
          class="pb-2 text-sm font-medium min-h-[44px]"
          :style="{
            borderBottom: activeTab === 'triggers' ? '2px solid var(--pb-accent)' : '2px solid transparent',
            color: activeTab === 'triggers' ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
          }"
        >
          Triggers
          <span
            v-if="triggersStore.triggers.length"
            class="ml-1 rounded-full px-1.5 py-0.5 text-xs"
            style="background-color: var(--pb-bg-elevated); color: var(--pb-text-muted)"
          >
            {{ triggersStore.triggers.length }}
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
    <template v-else-if="activeTab === 'triggers'">
      <FeatureHint
        storage-key="alerts-triggers"
        title="What are alert triggers?"
        :doc-href="docUrl('features/alerts/#alert-triggers')"
      >
        Triggers are routing rules: each one matches alerts by severity, source, or scope, then dispatches them to one or more channels. A channel without any trigger stays silent — useful when you want to reserve it for an escalation policy. Manage your channels separately in the <RouterLink to="/channels" class="text-pb-green-400 hover:underline">Channels</RouterLink> page.
      </FeatureHint>
      <TriggerManager />
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
