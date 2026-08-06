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
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ApiError } from '@/services/apiFetch'
import EditionBadge from '@/components/EditionBadge.vue'

/**
 * Renders a creation error. When it is an edition or quota refusal, the message
 * is composed here from the structured fields — resource, limit and the edition
 * that lifts the cap — instead of splitting the backend's English sentence,
 * which is what the four quota pages used to do.
 */
const props = defineProps<{
  /** The caught error, or a plain message for everything that is not an API refusal. */
  error: unknown
}>()

const apiError = computed(() => (props.error instanceof ApiError ? props.error : null))

const fallbackMessage = computed(() => {
  if (apiError.value) return apiError.value.message
  if (props.error instanceof Error) return props.error.message
  return typeof props.error === 'string' ? props.error : ''
})

const refusal = computed(() => {
  const e = apiError.value
  if (!e || !(e.isQuotaRefusal || e.isEditionRefusal)) return null
  return {
    message: e.message,
    requiredEdition: e.detail?.required_edition ?? null,
  }
})
</script>

<template>
  <div
    class="mb-3 rounded p-2 text-sm"
    :style="{
      backgroundColor: 'var(--mnt-status-down-bg)',
      color: 'var(--mnt-status-down)',
      borderRadius: 'var(--mnt-radius-sm)',
    }"
  >
    <template v-if="refusal">
      {{ refusal.message }}
      <span v-if="refusal.requiredEdition" class="ml-1 inline-flex items-center gap-1.5">
        <RouterLink
          :to="{ name: 'editions' }"
          class="font-medium underline transition-opacity hover:opacity-80"
          style="color: var(--mnt-accent)"
        >
          Available from
        </RouterLink>
        <EditionBadge :edition="refusal.requiredEdition" />
      </span>
    </template>
    <template v-else>
      {{ fallbackMessage }}
    </template>
  </div>
</template>
