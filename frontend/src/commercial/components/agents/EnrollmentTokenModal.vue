<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
  See internal/commercial/LICENSE.
-->

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import type { EnrollmentTokenCreated, InstallMode } from '@/services/agentApi'
import UiModal from '@/components/ui/UiModal.vue'
import UiButton from '@/components/ui/UiButton.vue'
import SegmentedToggle from '@/components/ui/SegmentedToggle.vue'
import InlineAlert from '@/components/ui/InlineAlert.vue'

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
const MODE_OPTIONS = MODES.map((m) => ({ value: m.id, label: m.label }))

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

function selectMode(mode: string) {
  if (!isValidMode(mode)) return
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

const open = ref(true)
watch(open, (value) => {
  if (!value) emit('close')
})
</script>

<template>
  <UiModal v-model:open="open" title="Enrollment Token" size="lg">
    <div class="space-y-5">
      <p class="text-sm text-mnt-muted">
        This token will <span class="text-mnt-primary font-semibold">never be shown again</span>.
        Copy the install command before closing.
      </p>

      <InlineAlert v-if="hasLocalWarning" severity="warning" title="Local address detected">
        The server URL resolves to a local address. Remote agents won't be able to connect.
        Set <code class="font-mono bg-mnt-elevated px-1 rounded">MAINTENANT_GRPC_URL</code>
        to a publicly reachable address.
      </InlineAlert>

      <!-- Token cleartext -->
      <div>
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-1">Token</p>
        <div class="flex items-center gap-2">
          <code
            class="flex-1 block rounded-lg bg-mnt-primary border border-mnt-default px-3 py-2 font-mono text-xs text-mnt-secondary break-all"
          >{{ token.token }}</code>
          <UiButton variant="secondary" size="sm" @click="copyText(token.token, 'token')">
            {{ copiedToken ? 'Copied!' : 'Copy' }}
          </UiButton>
        </div>
      </div>

      <!-- Install mode selector + command -->
      <div>
        <p class="text-[10px] text-mnt-muted font-bold uppercase tracking-widest mb-2">Install command</p>

        <SegmentedToggle
          :model-value="selectedMode"
          :options="MODE_OPTIONS"
          ariaLabel="Install mode"
          class="mb-2"
          @update:model-value="selectMode"
        />

        <!-- Template body -->
        <pre
          class="rounded-lg bg-mnt-primary border border-mnt-default px-3 py-2 font-mono text-xs text-mnt-secondary whitespace-pre overflow-auto max-h-80"
        >{{ currentTemplate }}</pre>

        <UiButton
          v-if="selectedMode !== 'standalone'"
          variant="secondary"
          class="mt-2 w-full"
          @click="copyText(currentTemplate, 'command')"
        >
          {{ copiedCommand ? 'Copied!' : 'Copy install command' }}
        </UiButton>
      </div>

      <!-- Expires -->
      <p class="text-xs text-mnt-muted">
        Expires: {{ new Date(token.expires_at).toLocaleString() }}
      </p>
    </div>

    <template #footer>
      <UiButton variant="primary" class="w-full" @click="open = false">
        Done
      </UiButton>
    </template>
  </UiModal>
</template>
