<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { AlertTriangle, RefreshCw } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    title?: string
    message: string
    retryable?: boolean
  }>(),
  { title: 'Something failed to load', retryable: false },
)

const emit = defineEmits<{ retry: [] }>()
</script>

<template>
  <div role="alert" class="flex flex-col items-center justify-center px-6 py-12 text-center">
    <span class="mb-3 inline-flex h-10 w-10 items-center justify-center rounded-full bg-pb-sev-incident">
      <AlertTriangle :size="20" class="text-pb-sev-incident" aria-hidden="true" />
    </span>
    <p class="text-sm font-semibold text-pb-primary">{{ title }}</p>
    <p class="mt-1 max-w-sm text-xs text-pb-secondary">{{ message }}</p>
    <button
      v-if="retryable"
      type="button"
      class="focus-ring mt-4 inline-flex items-center gap-1.5 rounded-lg border border-pb-default px-3 py-1.5 text-xs font-semibold text-pb-secondary transition-colors hover:text-pb-primary"
      @click="emit('retry')"
    >
      <RefreshCw :size="13" aria-hidden="true" />
      Retry
    </button>
  </div>
</template>
