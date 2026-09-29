<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { MonitorDot } from 'lucide-vue-next'
import EditionBadge from '@/components/EditionBadge.vue'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import type { Edition } from '@/services/editionApi'

const props = defineProps<{
  used: number
  limit: number
  /**
   * The edition that lifts this cap, taken from the refusal the server sent.
   * Absent when the caller has no refusal to hand — the dialog then falls back
   * to the direct-contact wording rather than naming a tier it guessed.
   */
  requiredEdition?: Edition | null
}>()

const emit = defineEmits<{
  close: []
}>()

const requiredEdition = computed(() => props.requiredEdition ?? null)

const mailto =
  'mailto:benjamin@kolapsis.com' +
  '?subject=' +
  encodeURIComponent('Maintenant: monitoring more hosts')

const open = ref(true)
watch(open, (value) => {
  if (!value) emit('close')
})
</script>

<template>
  <UiModal v-model:open="open" title="Host limit reached" size="sm">
    <div class="space-y-5">
      <div class="flex items-start gap-3">
        <div
          class="shrink-0 w-10 h-10 rounded-xl flex items-center justify-center"
          style="background: var(--mnt-bg-elevated); border: 1px solid var(--mnt-border-default)"
        >
          <MonitorDot :size="20" class="text-mnt-accent" />
        </div>
        <p class="text-sm text-mnt-muted">
          You're monitoring
          <span class="font-semibold text-mnt-secondary">{{ used }} of {{ limit }}</span>
          hosts on this server.
        </p>
      </div>

      <p v-if="requiredEdition" class="text-sm text-mnt-muted leading-relaxed">
        The
        <EditionBadge :edition="requiredEdition" class="mx-0.5 align-middle" />
        edition lifts this limit.
      </p>
      <p v-else class="text-sm text-mnt-muted leading-relaxed">
        Need to monitor more hosts? Get in touch directly and I'll help you set it up.
      </p>
    </div>

    <template #footer>
      <UiButton variant="secondary" @click="open = false">Close</UiButton>
      <RouterLink
        v-if="requiredEdition"
        :to="{ name: 'editions' }"
        class="inline-flex items-center justify-center rounded-lg px-4 py-2 text-sm font-semibold transition-opacity hover:opacity-90"
        :style="{
          backgroundColor: 'var(--mnt-accent)',
          color: 'var(--mnt-text-inverted)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
        @click="open = false"
      >
        Compare editions
      </RouterLink>
      <a
        v-else
        :href="mailto"
        class="inline-flex items-center justify-center rounded-lg px-4 py-2 text-sm font-semibold transition-opacity hover:opacity-90"
        :style="{
          backgroundColor: 'var(--mnt-accent)',
          color: 'var(--mnt-text-inverted)',
          borderRadius: 'var(--mnt-radius-md)',
        }"
      >
        Email benjamin@kolapsis.com
      </a>
    </template>
  </UiModal>
</template>
