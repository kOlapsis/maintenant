<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useStatusAdminStore } from '@/stores/statusAdmin'
import StatusComponentManager from '@/components/StatusComponentManager.vue'
import StatusIncidentManager from '@/components/StatusIncidentManager.vue'
import StatusSmtpConfig from '@/components/StatusSmtpConfig.vue'
import StatusMaintenanceManager from '@/components/StatusMaintenanceManager.vue'
import ProFeatureGate from '@/components/ProFeatureGate.vue'
import { useEdition } from '@/composables/useEdition'

const { hasFeature } = useEdition()

const store = useStatusAdminStore()
const activeTab = ref<'components' | 'incidents' | 'maintenance' | 'subscribers' | 'smtp'>('components')

onMounted(() => {
  store.fetchComponents()
  if (hasFeature('incidents')) store.fetchIncidents()
  if (hasFeature('maintenance_windows')) store.fetchMaintenance()
  if (hasFeature('subscribers')) store.fetchSubscribers()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-white">Status Page</h1>
      <p class="mt-1 text-sm" style="color: var(--pb-text-muted)">
        Manage the public status page components, incidents, and maintenance windows
      </p>
      <a
        href="/status"
        target="_blank"
        class="mt-1 inline-block text-sm transition-colors"
        style="color: var(--pb-accent)"
        @mouseenter="($event.target as HTMLElement).style.color = 'var(--pb-accent-hover)'"
        @mouseleave="($event.target as HTMLElement).style.color = 'var(--pb-accent)'"
      >
        View public status page &rarr;
      </a>
    </div>

    <!-- Tab navigation -->
    <div class="mb-4 border-b" style="border-color: var(--pb-border-default)">
      <nav class="-mb-px flex gap-6">
        <button
          v-for="tab in [
            { key: 'components', label: 'Components', count: store.components?.length ?? 0 },
            { key: 'incidents', label: 'Incidents', count: store.incidentsTotal },
            { key: 'maintenance', label: 'Maintenance', count: store.maintenance?.length ?? 0 },
            { key: 'subscribers', label: 'Subscribers', count: store.subscriberTotal },
            { key: 'smtp', label: 'SMTP', count: 0 },
          ]"
          :key="tab.key"
          @click="activeTab = tab.key as any"
          class="border-b-2 pb-2 text-sm font-medium transition-colors"
          :style="{
            borderColor: activeTab === tab.key ? 'var(--pb-accent)' : 'transparent',
            color: activeTab === tab.key ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
          }"
        >
          {{ tab.label }}
          <span
            v-if="tab.key === 'subscribers' && store.subscriberTotal"
            class="ml-1 rounded-full px-1.5 py-0.5 text-xs"
            style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
          >
            {{ store.subscriberConfirmed }}/{{ store.subscriberTotal }}
          </span>
          <span
            v-else-if="tab.count"
            class="ml-1 rounded-full px-1.5 py-0.5 text-xs"
            style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)"
          >
            {{ tab.count }}
          </span>
        </button>
      </nav>
    </div>

    <!-- Tab content -->
    <StatusComponentManager v-if="activeTab === 'components'" />
    <ProFeatureGate v-else-if="activeTab === 'incidents'" feature="incidents" title="Incident Management">
      <StatusIncidentManager />
    </ProFeatureGate>
    <ProFeatureGate v-else-if="activeTab === 'maintenance'" feature="maintenance_windows" title="Maintenance Windows">
      <StatusMaintenanceManager />
    </ProFeatureGate>
    <ProFeatureGate v-else-if="activeTab === 'subscribers'" feature="subscribers" title="Subscriber Notifications">
      <div
        class="rounded-lg border p-6"
        style="background: var(--pb-bg-surface); border-color: var(--pb-border-default)"
      >
        <h2 class="mb-3 text-lg font-semibold" style="color: var(--pb-text-primary)">Subscribers</h2>
        <div class="mb-3 flex gap-4">
          <div class="rounded-lg border px-4 py-2" style="border-color: var(--pb-border-default); background: var(--pb-bg-elevated)">
            <p class="text-2xl font-bold" style="color: var(--pb-text-primary)">{{ store.subscriberTotal }}</p>
            <p class="text-xs" style="color: var(--pb-text-muted)">Total</p>
          </div>
          <div class="rounded-lg border px-4 py-2" style="border-color: var(--pb-border-default); background: var(--pb-bg-elevated)">
            <p class="text-2xl font-bold" style="color: var(--pb-status-ok)">{{ store.subscriberConfirmed }}</p>
            <p class="text-xs" style="color: var(--pb-text-muted)">Confirmed</p>
          </div>
        </div>
        <div v-if="(store.subscribers?.length ?? 0) === 0" class="text-center">
          <p class="text-sm" style="color: var(--pb-text-muted)">No subscribers yet</p>
        </div>
        <div v-else class="space-y-1">
          <div
            v-for="sub in store.subscribers"
            :key="sub.id"
            class="flex items-center justify-between rounded px-3 py-1.5 text-sm transition-colors"
            style="color: var(--pb-text-secondary)"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--pb-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          >
            <span>{{ sub.email }}</span>
            <span
              class="rounded px-1.5 py-0.5 text-xs"
              :style="{
                background: sub.confirmed ? 'var(--pb-status-ok-bg)' : 'var(--pb-status-warn-bg)',
                color: sub.confirmed ? 'var(--pb-status-ok)' : 'var(--pb-status-warn)',
              }"
            >
              {{ sub.confirmed ? 'confirmed' : 'pending' }}
            </span>
          </div>
        </div>
      </div>
    </ProFeatureGate>
    <ProFeatureGate v-else-if="activeTab === 'smtp'" feature="smtp" title="SMTP Configuration">
      <StatusSmtpConfig />
    </ProFeatureGate>
  </div>
</template>
