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
import { useEdition } from '@/composables/useEdition'
import { Lock, Sparkles } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import EditionBadge from '@/components/EditionBadge.vue'

const props = defineProps<{
  feature: string
  title?: string
  description?: string
}>()

const { hasFeature, requiredEditionFor } = useEdition()
const enabled = computed(() => hasFeature(props.feature))

/**
 * The edition this specific capability needs, read from the backend registry.
 * Null when the engine did not declare it — in that case the placeholder stays
 * generic rather than naming a tier it would be guessing at.
 */
const required = computed(() => requiredEditionFor(props.feature))
</script>

<template>
  <slot v-if="enabled" />
  <template v-else-if="!enabled">
    <slot name="placeholder">
      <!-- Default placeholder when no custom one is provided -->
      <div
        v-if="title"
        class="relative w-full rounded-xl border border-mnt-default bg-mnt-surface px-5 py-5"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <div class="flex items-center gap-2 mb-1">
              <Sparkles class="h-3.5 w-3.5 text-mnt-muted shrink-0" />
              <span class="text-sm font-semibold text-mnt-primary">{{ title }}</span>
            </div>
            <p v-if="description" class="text-xs leading-relaxed text-mnt-muted pl-5.5">
              {{ description }}
            </p>
          </div>
          <router-link :to="{ name: 'editions' }" class="flex items-center gap-1.5 shrink-0 mt-0.5">
            <Lock class="h-3 w-3 text-mnt-muted" />
            <EditionBadge v-if="required" :edition="required" />
            <span
              v-else
              class="rounded-full border border-mnt-default px-2.5 py-0.5 text-[10px] font-semibold text-mnt-muted"
            >
              Locked
            </span>
          </router-link>
        </div>
      </div>
    </slot>
  </template>
</template>
