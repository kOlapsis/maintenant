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
import { ref } from 'vue'
import type { EnrollmentTokenCreated } from '@/services/agentApi'

const props = defineProps<{
  token: EnrollmentTokenCreated
}>()

const emit = defineEmits<{
  close: []
}>()

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
        class="relative mx-4 w-full max-w-lg overflow-hidden"
        :style="{
          backgroundColor: 'var(--pb-bg-surface)',
          border: '1px solid var(--pb-border-default)',
          borderRadius: 'var(--pb-radius-lg)',
          boxShadow: 'var(--pb-shadow-elevated)',
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
              class="text-lg font-semibold text-white"
            >
              Enrollment Token
            </h2>
            <p class="mt-1 text-sm text-slate-400">
              This token will <span class="text-white font-semibold">never be shown again</span>.
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
              Set <code class="font-mono bg-yellow-500/10 px-1 rounded">MAINTENANT_GRPC_PUBLIC_URL</code>
              to a publicly reachable address.
            </p>
          </div>

          <!-- Token cleartext -->
          <div>
            <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">Token</p>
            <div class="flex items-center gap-2">
              <code
                class="flex-1 block rounded-lg bg-[#0B0E13] border border-slate-800 px-3 py-2 font-mono text-xs text-slate-300 break-all"
              >{{ token.token }}</code>
              <button
                type="button"
                class="shrink-0 rounded-lg border border-slate-700 bg-[#0B0E13] px-3 py-2 text-xs text-slate-300 hover:bg-slate-800 transition-colors"
                @click="copyText(token.token, 'token')"
              >
                {{ copiedToken ? 'Copied!' : 'Copy' }}
              </button>
            </div>
          </div>

          <!-- Install command -->
          <div>
            <p class="text-[10px] text-slate-500 font-bold uppercase tracking-widest mb-1">Install command</p>
            <div
              class="rounded-lg bg-[#0B0E13] border border-slate-800 px-3 py-2 font-mono text-xs text-slate-300 break-all"
            >{{ token.install_command }}</div>
            <button
              type="button"
              class="mt-2 w-full rounded-lg border border-slate-700 bg-[#0B0E13] px-4 py-2 text-sm font-medium text-slate-300 hover:bg-slate-800 transition-colors"
              @click="copyText(token.install_command, 'command')"
            >
              {{ copiedCommand ? 'Copied!' : 'Copy install command' }}
            </button>
          </div>

          <!-- Expires -->
          <p class="text-xs text-slate-500">
            Expires: {{ new Date(token.expires_at).toLocaleString() }}
          </p>

          <!-- Done -->
          <button
            type="button"
            class="w-full rounded-lg bg-pb-green-600 px-4 py-2 text-sm font-semibold text-white hover:opacity-90 transition-opacity"
            @click="emit('close')"
          >
            Done
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
