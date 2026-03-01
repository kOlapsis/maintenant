<script setup lang="ts">
import { onMounted, onUnmounted, computed } from 'vue'
import { useEndpointsStore } from '@/stores/endpoints'
import { useContainersStore } from '@/stores/containers'
import EndpointCard from '@/components/EndpointCard.vue'
import { AlertTriangle, Globe } from 'lucide-vue-next'

const store = useEndpointsStore()
const containers = useContainersStore()

const isK8s = computed(() => containers.runtimeName === 'kubernetes')
const labelOrAnnotation = computed(() => isK8s.value ? 'annotation' : 'label')

onMounted(() => {
  store.fetchEndpoints()
  store.connectSSE()
})

onUnmounted(() => {
  store.disconnectSSE()
})
</script>

<template>
  <div class="overflow-y-auto p-6">
    <div class="mx-auto max-w-7xl">
    <div class="mb-6">
      <h1 class="text-2xl font-black text-white">Endpoints</h1>
      <p class="mt-1 text-sm text-slate-500">
        HTTP/TCP endpoint health checks
      </p>
    </div>

    <!-- Config errors -->
    <div
      v-if="store.configErrors.length > 0"
      class="mb-6 rounded-2xl p-4 bg-amber-500/10 border border-amber-500/30"
    >
      <div class="flex items-start gap-3">
        <AlertTriangle :size="20" class="text-amber-500 shrink-0 mt-0.5" />
        <div>
          <h3 class="text-sm font-medium text-amber-400">Label configuration errors</h3>
          <ul class="mt-1 space-y-0.5 text-sm text-slate-400">
            <li v-for="(err, i) in store.configErrors" :key="i">
              <strong>{{ err.container_name }}</strong> ({{ err.label_key }}): {{ err.error }}
            </li>
          </ul>
        </div>
      </div>
    </div>

    <!-- Status summary -->
    <div class="mb-6 flex gap-3 text-sm">
      <span class="rounded-full bg-emerald-500/15 text-emerald-400 px-3 py-1 font-medium">
        {{ store.statusCounts.up }} up
      </span>
      <span class="rounded-full bg-rose-500/15 text-rose-400 px-3 py-1 font-medium">
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
        class="rounded-lg border border-slate-800 bg-slate-900 text-white px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500 min-h-[44px]"
      >
        <option value="">All statuses</option>
        <option value="up">Up</option>
        <option value="down">Down</option>
        <option value="unknown">Unknown</option>
      </select>

      <select
        v-model="store.typeFilter"
        class="rounded-lg border border-slate-800 bg-slate-900 text-white px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500 min-h-[44px]"
      >
        <option value="">All types</option>
        <option value="http">HTTP</option>
        <option value="tcp">TCP</option>
      </select>

      <select
        v-model="store.containerFilter"
        class="rounded-lg border border-slate-800 bg-slate-900 text-white px-3 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500 min-h-[44px]"
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
      class="rounded-2xl p-4 text-sm bg-rose-500/10 border border-rose-500/30 text-rose-400"
    >
      {{ store.error }}
    </div>

    <!-- Content area with persistent background hint -->
    <div v-else class="relative min-h-[300px]">
      <!-- Background hint — always visible -->
      <div class="flex flex-col items-center justify-center py-16 text-center">
        <div class="p-4 bg-slate-900 rounded-2xl mb-4">
          <Globe :size="48" class="text-slate-600" />
        </div>
        <p class="text-sm mb-2 max-w-md text-slate-500">
          Monitor HTTP and TCP endpoints by adding {{ labelOrAnnotation }}s to your {{ isK8s ? 'pods' : 'containers' }}.
        </p>
        <p class="text-sm max-w-md text-slate-500">
          Add the <code class="rounded-md px-1.5 py-0.5 text-xs bg-slate-900 text-slate-300">pulseboard.endpoint.http</code>
          or <code class="rounded-md px-1.5 py-0.5 text-xs bg-slate-900 text-slate-300">pulseboard.endpoint.tcp</code>
          {{ labelOrAnnotation }} with the target URL.
        </p>
      </div>

      <!-- Endpoint grid — overlays on top -->
      <div
        v-if="store.filteredEndpoints.length > 0"
        class="absolute inset-0 grid gap-4 sm:grid-cols-2 lg:grid-cols-3 content-start bg-[#0f1115]"
      >
        <EndpointCard
          v-for="ep in store.filteredEndpoints"
          :key="ep.id"
          :endpoint="ep"
        />
      </div>
    </div>
  </div>
  </div>
</template>
