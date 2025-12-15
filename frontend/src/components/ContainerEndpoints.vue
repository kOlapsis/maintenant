<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listEndpoints, type Endpoint } from '@/services/endpointApi'
import EndpointStatusBadge from './EndpointStatusBadge.vue'

const props = defineProps<{
  containerName: string
}>()

const endpoints = ref<Endpoint[]>([])
const loading = ref(false)

onMounted(async () => {
  loading.value = true
  try {
    const res = await listEndpoints({ container: props.containerName })
    endpoints.value = res.endpoints || []
  } catch {
    // Silently ignore — this is a summary widget
  } finally {
    loading.value = false
  }
})

function formatResponseTime(ms: number | undefined): string {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
</script>

<template>
  <div v-if="endpoints.length > 0" class="mt-3 space-y-1.5">
    <div class="text-xs font-medium text-gray-500 uppercase tracking-wide">
      Endpoints
    </div>
    <div
      v-for="ep in endpoints"
      :key="ep.id"
      class="flex items-center justify-between rounded bg-gray-50 px-2 py-1 text-xs"
    >
      <div class="flex items-center gap-2 min-w-0">
        <span
          class="font-mono uppercase"
          :class="{
            'text-blue-600': ep.endpoint_type === 'http',
            'text-purple-600': ep.endpoint_type === 'tcp',
          }"
        >
          {{ ep.endpoint_type }}
        </span>
        <span class="truncate text-gray-700">{{ ep.target }}</span>
      </div>
      <div class="flex items-center gap-2 ml-2 shrink-0">
        <span class="text-gray-400">
          {{ formatResponseTime(ep.last_response_time_ms) }}
        </span>
        <EndpointStatusBadge :status="ep.status" />
      </div>
    </div>
  </div>
</template>
