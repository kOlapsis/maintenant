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
import { MonitorDot } from 'lucide-vue-next'

defineProps<{
  used: number
  limit: number
}>()

const emit = defineEmits<{
  close: []
}>()

const mailto =
  'mailto:benjamin@kolapsis.com' +
  '?subject=' +
  encodeURIComponent('Maintenant — monitoring more hosts')
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[10001] flex items-center justify-center"
      @keydown.esc="emit('close')"
    >
      <div class="fixed inset-0 bg-black/70 backdrop-blur-sm" @click="emit('close')" />

      <div
        class="relative mx-4 w-full max-w-md overflow-hidden"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-lg)',
          boxShadow: 'var(--mnt-shadow-elevated)',
        }"
        role="dialog"
        aria-modal="true"
        aria-labelledby="host-limit-title"
      >
        <div class="p-6 space-y-5">
          <div class="flex items-start gap-3">
            <div
              class="shrink-0 w-10 h-10 rounded-xl flex items-center justify-center"
              style="background: var(--mnt-bg-elevated); border: 1px solid var(--mnt-border-default)"
            >
              <MonitorDot :size="20" class="text-mnt-accent" />
            </div>
            <div>
              <h2 id="host-limit-title" class="text-lg font-semibold text-mnt-primary">
                Host limit reached
              </h2>
              <p class="mt-1 text-sm text-mnt-muted">
                You're monitoring
                <span class="font-semibold text-mnt-secondary">{{ used }} of {{ limit }}</span>
                hosts on this server.
              </p>
            </div>
          </div>

          <p class="text-sm text-mnt-muted leading-relaxed">
            Need to monitor more hosts? Get in touch directly and I'll help you set it up.
          </p>

          <div class="flex flex-col gap-2">
            <a
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
            <button
              type="button"
              class="inline-flex items-center justify-center rounded-lg px-4 py-2 text-sm font-medium transition-colors"
              :style="{
                backgroundColor: 'var(--mnt-bg-elevated)',
                color: 'var(--mnt-text-secondary)',
                borderRadius: 'var(--mnt-radius-md)',
              }"
              @click="emit('close')"
            >
              Close
            </button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
