<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
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
import TabNav, { type TabNavItem } from '@/components/ui/TabNav.vue'
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

const tabItems = computed<TabNavItem[]>(() => [
  { value: 'history', label: 'History' },
  { value: 'triggers', label: 'Triggers', count: triggersStore.triggers.length || undefined },
  { value: 'silence', label: 'Silence Rules', count: store.activeSilenceCount || undefined, countTone: 'warn' },
])

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
      <h1 class="text-2xl font-black text-mnt-primary">Alerts</h1>
      <p class="mt-1 text-sm text-mnt-muted">
        Alert history, routing triggers, and silence rules
      </p>
    </div>

    <!-- Active alerts -->
    <div class="mb-6">
      <h2 class="mb-2 text-sm font-medium" style="color: var(--mnt-text-secondary)">Active Alerts</h2>
      <ActiveAlerts />
    </div>

    <!-- Tab navigation -->
    <div class="mb-4">
      <TabNav v-model="activeTab" :items="tabItems" ariaLabel="Alerts sections" />
    </div>

    <!-- Tab content -->
    <AlertList v-if="activeTab === 'history'" />
    <template v-else-if="activeTab === 'triggers'">
      <FeatureHint
        storage-key="alerts-triggers"
        title="What are alert triggers?"
        :doc-href="docUrl('features/alerts/#alert-triggers')"
      >
        Triggers are routing rules: each one matches alerts by severity, source, or scope, then dispatches them to one or more channels. A channel without any trigger stays silent — useful when you want to reserve it for an escalation policy. Manage your channels separately in the <RouterLink to="/channels" class="text-mnt-green-400 hover:underline">Channels</RouterLink> page.
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
