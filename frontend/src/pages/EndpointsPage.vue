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
import { ref, onMounted, onUnmounted, computed } from 'vue'
import { useEndpointsStore } from '@/stores/endpoints'
import { useContainersStore } from '@/stores/containers'
import { useEdition } from '@/composables/useEdition'
import { createEndpoint } from '@/services/endpointApi'
import EndpointCard from '@/components/EndpointCard.vue'
import { Globe } from 'lucide-vue-next'
import InlineAlert from '@/components/ui/InlineAlert.vue'
import FeatureHint from '@/components/ui/FeatureHint.vue'
import { docUrl } from '@/utils/docs'

const store = useEndpointsStore()
const containers = useContainersStore()
const { getQuota, reload } = useEdition()
const quota = getQuota('endpoints')

const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')

const showCreateForm = ref(false)
const createError = ref<string | null>(null)
const creating = ref(false)

const isQuotaError = computed(() => {
  return createError.value?.includes('Upgrade to Pro') || false
})

const form = ref({
  name: '',
  target: '',
  endpoint_type: 'http' as 'http' | 'tcp',
  interval: '30s',
})

const intervalPresets = [
  { label: '10s', value: '10s' },
  { label: '30s', value: '30s' },
  { label: '1m', value: '1m0s' },
  { label: '5m', value: '5m0s' },
  { label: '15m', value: '15m0s' },
]

function resetForm() {
  form.value = { name: '', target: '', endpoint_type: 'http', interval: '30s' }
  createError.value = null
}

async function handleCreate() {
  createError.value = null
  creating.value = true
  try {
    await createEndpoint({
      name: form.value.name,
      target: form.value.target,
      endpoint_type: form.value.endpoint_type,
      interval: form.value.interval,
    })
    // HTTPS endpoints get an auto-detected cert monitor (source='auto')
    // at first check — it is tied to the endpoint and not counted against
    // the standalone cert quota.

    showCreateForm.value = false
    resetForm()
    store.fetchEndpoints()
    reload()
  } catch (e) {
    createError.value = e instanceof Error ? e.message : 'Failed to create endpoint'
  } finally {
    creating.value = false
  }
}

onMounted(() => {
  store.fetchEndpoints()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-3 sm:p-6">
    <div class="mx-auto max-w-7xl">
    <div class="mb-6 flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-black text-pb-primary">Endpoints</h1>
        <p class="mt-1 text-sm text-slate-500">
          HTTP/TCP endpoint health checks
        </p>
      </div>
      <div class="flex items-center gap-2">
        <span
          v-if="!quota.isUnlimited"
          class="rounded-full px-2.5 py-1 text-xs font-medium"
          :style="{
            backgroundColor: quota.isAtLimit ? 'var(--pb-status-down-bg)' : quota.nearLimit ? 'var(--pb-status-warn-bg)' : 'var(--pb-bg-elevated)',
            color: quota.isAtLimit ? 'var(--pb-status-down)' : quota.nearLimit ? 'var(--pb-status-warn)' : 'var(--pb-text-secondary)',
          }"
        >
          {{ quota.used }}/{{ quota.limit }}
        </span>
        <router-link
          v-if="quota.nearLimit && !quota.isAtLimit"
          :to="{ name: 'pro-edition' }"
          class="text-xs font-medium transition-opacity hover:opacity-80"
          style="color: var(--pb-accent)"
        >
          Upgrade
        </router-link>
        <button
          class="min-h-[44px]"
          :disabled="quota.isAtLimit"
          :title="quota.isAtLimit ? `Community edition limited to ${quota.limit} endpoints` : ''"
          :style="{
            borderRadius: 'var(--pb-radius-lg)',
            backgroundColor: 'var(--pb-accent)',
            color: 'var(--pb-text-inverted)',
            padding: '0.5rem 1rem',
            fontSize: '0.875rem',
            fontWeight: '500',
            opacity: quota.isAtLimit ? '0.5' : '1',
            cursor: quota.isAtLimit ? 'not-allowed' : 'pointer',
          }"
          @click="showCreateForm = !showCreateForm; if (!showCreateForm) resetForm()"
        >
          {{ showCreateForm ? 'Cancel' : 'New Endpoint' }}
        </button>
      </div>
    </div>

    <FeatureHint
      storage-key="endpoints"
      title="Define HTTP/TCP checks with labels"
      :doc-href="docUrl('features/endpoints/#quick-start')"
    >
      Declare endpoints directly on your {{ isK8s ? 'pods' : 'containers' }} with
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.endpoint.http</code>
      or
      <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.endpoint.tcp</code>,
      and tune the interval, failure/recovery thresholds, expected status codes, or TLS verification via sibling {{ labelOrAnnotation }}s. Use indexed labels
      (<code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.endpoint.0.*</code>)
      to monitor multiple endpoints from the same {{ isK8s ? 'pod' : 'container' }}.
    </FeatureHint>

    <!-- Create form -->
    <div
      v-if="showCreateForm"
      class="mb-6 p-4"
      :style="{
        backgroundColor: 'var(--pb-bg-surface)',
        border: '1px solid var(--pb-border-default)',
        borderRadius: 'var(--pb-radius-lg)',
      }"
    >
      <h3 class="mb-3 text-sm font-semibold" :style="{ color: 'var(--pb-text-primary)' }">Create Endpoint Monitor</h3>
      <div
        v-if="createError"
        class="mb-3 rounded p-2 text-sm"
        :style="{
          backgroundColor: 'var(--pb-status-down-bg)',
          color: 'var(--pb-status-down)',
          borderRadius: 'var(--pb-radius-sm)',
        }"
      >
        <template v-if="isQuotaError">
          {{ createError.split('Upgrade to Pro')[0] }}
          <a
            href="/pro-edition"
            class="font-medium underline transition-opacity hover:opacity-80"
            style="color: #a78bfa"
          >
            Upgrade to Pro
          </a>
          {{ createError.split('Upgrade to Pro')[1] }}
        </template>
        <template v-else>
          {{ createError }}
        </template>
      </div>
      <form class="flex flex-col gap-3" @submit.prevent="handleCreate">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Name</label>
            <input
              v-model="form.name"
              type="text"
              placeholder="e.g., Production API"
              :style="{
                width: '100%',
                borderRadius: 'var(--pb-radius-md)',
                border: '1px solid var(--pb-border-default)',
                backgroundColor: 'var(--pb-bg-elevated)',
                color: 'var(--pb-text-primary)',
                padding: '0.375rem 0.75rem',
                fontSize: '0.875rem',
                minHeight: '44px',
              }"
              required
            />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Type</label>
            <div class="flex gap-2">
              <button
                v-for="t in (['http', 'tcp'] as const)"
                :key="t"
                type="button"
                class="flex-1 rounded-lg px-3 py-2 text-sm font-medium transition min-h-[44px]"
                :style="{
                  border: form.endpoint_type === t
                    ? '1px solid var(--pb-accent)'
                    : '1px solid var(--pb-border-default)',
                  backgroundColor: form.endpoint_type === t
                    ? 'var(--pb-accent)'
                    : 'var(--pb-bg-elevated)',
                  color: form.endpoint_type === t
                    ? 'var(--pb-text-inverted)'
                    : 'var(--pb-text-secondary)',
                  textTransform: 'uppercase',
                }"
                @click="form.endpoint_type = t"
              >
                {{ t }}
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">
            {{ form.endpoint_type === 'http' ? 'URL' : 'Host:Port' }}
          </label>
          <input
            v-model="form.target"
            type="text"
            :placeholder="form.endpoint_type === 'http' ? 'https://example.com/health' : 'db.example.com:5432'"
            :style="{
              width: '100%',
              borderRadius: 'var(--pb-radius-md)',
              border: '1px solid var(--pb-border-default)',
              backgroundColor: 'var(--pb-bg-elevated)',
              color: 'var(--pb-text-primary)',
              padding: '0.375rem 0.75rem',
              fontSize: '0.875rem',
              fontFamily: 'monospace',
              minHeight: '44px',
            }"
            required
          />
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium" :style="{ color: 'var(--pb-text-secondary)' }">Check Interval</label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="preset in intervalPresets"
              :key="preset.value"
              type="button"
              class="rounded-full px-3 py-1 text-xs font-medium transition"
              :style="{
                border: form.interval === preset.value
                  ? '1px solid var(--pb-accent)'
                  : '1px solid var(--pb-border-default)',
                backgroundColor: form.interval === preset.value
                  ? 'var(--pb-accent)'
                  : 'transparent',
                color: form.interval === preset.value
                  ? 'var(--pb-text-inverted)'
                  : 'var(--pb-text-secondary)',
              }"
              @click="form.interval = preset.value"
            >
              {{ preset.label }}
            </button>
          </div>
        </div>

        <button
          type="submit"
          :disabled="creating"
          :style="{
            alignSelf: 'flex-start',
            borderRadius: 'var(--pb-radius-lg)',
            backgroundColor: 'var(--pb-accent)',
            color: 'var(--pb-text-inverted)',
            padding: '0.5rem 1rem',
            fontSize: '0.875rem',
            fontWeight: '500',
            opacity: creating ? '0.6' : '1',
          }"
        >
          {{ creating ? 'Creating...' : 'Create' }}
        </button>
      </form>
    </div>

    <!-- Config errors -->
    <InlineAlert
      v-if="store.configErrors.length > 0"
      severity="warning"
      :tag="`${store.configErrors.length} ${store.configErrors.length > 1 ? 'ERRORS' : 'ERROR'}`"
      class="mb-6 config-errors"
    >
      <template #title>Label configuration errors</template>
      <ul class="space-y-1">
        <li v-for="(err, i) in store.configErrors" :key="i" class="flex items-start gap-2">
          <span class="bullet mt-1.5 h-1 w-1 shrink-0 rounded-full" />
          <span>
            <strong class="font-semibold" style="color: var(--pb-alert-warn-title)">{{ err.container_name }}</strong>
            <span class="mx-1 opacity-70">({{ err.label_key }})</span>
            <span>{{ err.error }}</span>
          </span>
        </li>
      </ul>
    </InlineAlert>

    <!-- Status summary -->
    <div class="mb-6 flex gap-3 text-sm">
      <span class="rounded-full bg-pb-status-ok text-pb-status-ok px-3 py-1 font-medium">
        {{ store.statusCounts.up }} up
      </span>
      <span class="rounded-full bg-pb-status-down text-pb-status-down px-3 py-1 font-medium">
        {{ store.statusCounts.down }} down
      </span>
      <span class="rounded-full bg-slate-800 text-slate-400 px-3 py-1 font-medium">
        {{ store.statusCounts.unknown }} unknown
      </span>
    </div>

    <!-- Filters -->
    <div class="mb-6 flex flex-wrap gap-3">
      <select
        v-model="store.statusFilter"
        class="rounded-lg border px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-pb-green-500 min-h-[44px]"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
      >
        <option value="">All statuses</option>
        <option value="up">Up</option>
        <option value="down">Down</option>
        <option value="unknown">Unknown</option>
      </select>

      <select
        v-model="store.typeFilter"
        class="rounded-lg border px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-pb-green-500 min-h-[44px]"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
      >
        <option value="">All types</option>
        <option value="http">HTTP</option>
        <option value="tcp">TCP</option>
      </select>

      <select
        v-model="store.containerFilter"
        class="rounded-lg border px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-pb-green-500 min-h-[44px]"
        style="background: var(--pb-bg-elevated); border-color: var(--pb-border-default); color: var(--pb-text-secondary)"
      >
        <option value="">All containers</option>
        <option
          v-for="name in [...store.endpointsByContainer.keys()]"
          :key="name"
          :value="name"
        >
          {{ name }}
        </option>
      </select>
    </div>

    <!-- Loading -->
    <div v-if="store.loading" class="py-12 text-center text-slate-500">
      Loading endpoints...
    </div>

    <!-- Error -->
    <div
      v-else-if="store.error"
      class="rounded-2xl p-4 text-sm bg-pb-status-down border border-rose-500/30 text-pb-status-down"
    >
      {{ store.error }}
    </div>

    <!-- Content area with persistent background hint -->
    <div v-else class="relative min-h-[300px]">
      <!-- Background hint — always visible -->
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <div class="p-4 rounded-2xl mb-4" style="background: var(--pb-bg-elevated)">
          <Globe :size="48" class="text-slate-600" />
        </div>
        <p class="text-sm mb-2 max-w-md text-slate-500">
          Monitor HTTP and TCP endpoints by adding {{ labelOrAnnotation }}s to your {{ isK8s ? 'pods' : 'containers' }},
          or create standalone monitors using the button above.
        </p>
        <p class="text-sm max-w-md text-slate-500">
          Add the <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.endpoint.http</code>
          or <code class="rounded-md px-1.5 py-0.5 text-xs font-mono" style="background: var(--pb-bg-elevated); color: var(--pb-text-secondary)">maintenant.endpoint.tcp</code>
          {{ labelOrAnnotation }} with the target URL.
        </p>
      </div>

      <!-- Endpoint grid — overlays on top -->
      <div
        v-if="store.filteredEndpoints.length > 0"
        class="absolute inset-0 grid gap-4 sm:grid-cols-2 lg:grid-cols-3 content-start bg-pb-primary"
      >
        <EndpointCard
          v-for="ep in store.filteredEndpoints"
          :key="ep.id"
          :endpoint="ep"
          @deleted="store.fetchEndpoints(); reload()"
        />
      </div>
    </div>
  </div>
  </div>
</template>

<style scoped>
.config-errors .bullet {
  background: var(--pb-alert-warn-dot);
  opacity: 0.7;
}
</style>
