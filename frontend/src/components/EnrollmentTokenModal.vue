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
import { computed, onMounted, ref } from 'vue'
import type { EnrollmentTokenCreated, InstallMode } from '@/services/agentApi'

const props = defineProps<{
  token: EnrollmentTokenCreated
}>()

const emit = defineEmits<{
  close: []
}>()

const STORAGE_KEY = 'mnt:enrollment-modal-mode'
const MODES: Array<{ id: InstallMode; label: string }> = [
  { id: 'docker_run', label: 'Docker run' },
  { id: 'docker_compose', label: 'Compose' },
  { id: 'kubernetes', label: 'Kubernetes' },
  { id: 'standalone', label: 'Standalone (soon)' },
]

function isValidMode(value: unknown): value is InstallMode {
  return typeof value === 'string' && MODES.some((m) => m.id === value)
}

const selectedMode = ref<InstallMode>('docker_run')

onMounted(() => {
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY) ?? window.localStorage.getItem('pb:enrollment-modal-mode')
    if (isValidMode(saved)) {
      selectedMode.value = saved
    }
  } catch {
    /* localStorage unavailable — keep default */
  }
})

function selectMode(mode: InstallMode) {
  selectedMode.value = mode
  try {
    window.localStorage.setItem(STORAGE_KEY, mode)
  } catch {
    /* ignore */
  }
}

const currentTemplate = computed(() => props.token.install_templates[selectedMode.value] ?? '')

const copiedCommand = ref(false)
const copiedToken = ref(false)

function copyText(text: string, which: 'command' | 'token') {
  navigator.clipboard.writeText(text).then(() => {
    if (which === 'command') {
      copiedCommand.value = true
      setTimeout(() => { copiedCommand.value = false }, 2000)
    } else {
      copiedToken.value = true
      setTimeout(() => { copiedToken.value = false }, 2000)
    }
  })
}

const hasLocalWarning = props.token.warnings?.includes('public_url_appears_local') ?? false
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-[10001] flex items-center justify-center"
      @keydown.esc="emit('close')"
    >
      <div
        class="fixed inset-0 bg-black/70 backdrop-blur-sm"
      />

      <div
        class="relative mx-4 w-full max-w-2xl overflow-hidden"
        :style="{
          backgroundColor: 'var(--mnt-bg-surface)',
          border: '1px solid var(--mnt-border-default)',
          borderRadius: 'var(--mnt-radius-lg)',
          boxShadow: 'var(--mnt-shadow-elevated)',
        }"
        role="dialog"
        aria-modal="true"
        aria-labelledby="token-modal-title"
      >
        <div class="p-6 space-y-5">
          <!-- Header -->
          <div>
            <h2
              id="token-modal-title"
              class="text-lg font-semibold text-mnt-primary"
            >
              Enrollment Token
            </h2>
            <p class="mt-1 text-sm text-mnt-muted">
              This token will <span class="text-mnt-primary font-semibold">never be shown again</span>.
              Copy the install command before closing.
            </p>
          </div>

          <!-- Local URL warning -->
          <div
            v-if="hasLocalWarning"
            class="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-4 py-3"
          >
            <p class="text-sm text-yellow-400 font-semibold">Local address detected</p>
            <p class="mt-1 text-xs text-yellow-300/80">
              The server URL resolves to a local address. Remote agents won't be able to connect.
              Set <code class="font-mono bg-yellow-500/10 px-1 rounded">MAINTENANT_GRPC_URL</code>
              to a publicly reachable address.
            </p>
          </div>

          <!-- Token cleartext -->
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">Token</p>
            <div class="flex items-center gap-2">
              <code
                class="flex-1 block rounded-lg bg-mnt-primary border border-mnt-default px-3 py-2 font-mono text-xs text-mnt-secondary break-all"
              >{{ token.token }}</code>
              <button
                type="button"
                class="shrink-0 rounded-lg border border-mnt-default bg-mnt-primary px-3 py-2 text-xs text-mnt-secondary hover:bg-mnt-elevated transition-colors"
                @click="copyText(token.token, 'token')"
              >
                {{ copiedToken ? 'Copied!' : 'Copy' }}
              </button>
            </div>
          </div>

          <!-- Install mode selector + command -->
          <div>
            <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Install command</p>

            <!-- Segmented control -->
            <div
              role="tablist"
              aria-label="Install mode"
              class="flex flex-wrap gap-1 rounded-lg border border-mnt-default bg-mnt-primary p-1 mb-2"
            >
              <button
                v-for="mode in MODES"
                :key="mode.id"
                type="button"
                role="tab"
                :aria-selected="selectedMode === mode.id"
                class="flex-1 min-w-[5rem] rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                :class="
                  selectedMode === mode.id
                    ? 'bg-mnt-elevated text-mnt-primary'
                    : 'text-mnt-muted hover:bg-mnt-elevated hover:text-mnt-secondary'
                "
                @click="selectMode(mode.id)"
              >
                {{ mode.label }}
              </button>
            </div>

            <!-- Template body -->
            <pre
              class="rounded-lg bg-mnt-primary border border-mnt-default px-3 py-2 font-mono text-xs text-mnt-secondary whitespace-pre overflow-auto max-h-80"
            >{{ currentTemplate }}</pre>

            <button
              v-if="selectedMode !== 'standalone'"
              type="button"
              class="mt-2 w-full rounded-lg border border-mnt-default bg-mnt-primary px-4 py-2 text-sm font-medium text-mnt-secondary hover:bg-mnt-elevated transition-colors"
              @click="copyText(currentTemplate, 'command')"
            >
              {{ copiedCommand ? 'Copied!' : 'Copy install command' }}
            </button>
          </div>

          <!-- Expires -->
          <p class="text-xs text-mnt-muted">
            Expires: {{ new Date(token.expires_at).toLocaleString() }}
          </p>

          <!-- Done -->
          <button
            type="button"
            class="w-full rounded-lg bg-mnt-green-600 px-4 py-2 text-sm font-semibold text-mnt-primary hover:opacity-90 transition-opacity"
            @click="emit('close')"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
