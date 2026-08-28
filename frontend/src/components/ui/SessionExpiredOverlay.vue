<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
  or a commercial license. See COMMERCIAL-LICENSE.md.
-->
<script setup lang="ts">
import { LogIn, ShieldAlert } from 'lucide-vue-next'
import { reauthStalled, sessionExpired, startReauth } from '@/services/authGuard'
</script>

<template>
  <div
    v-if="sessionExpired"
    role="alertdialog"
    aria-modal="true"
    aria-labelledby="session-expired-title"
    class="session-scrim fixed inset-0 z-[100] flex items-center justify-center px-6 backdrop-blur-sm"
  >
    <div
      class="w-full max-w-sm rounded-xl border border-mnt-default bg-mnt-surface p-6 text-center shadow-lg"
    >
      <span
        class="mb-3 inline-flex h-10 w-10 items-center justify-center rounded-full bg-mnt-sev-warning"
      >
        <ShieldAlert :size="20" class="text-mnt-sev-warning" aria-hidden="true" />
      </span>
      <p id="session-expired-title" class="text-sm font-semibold text-mnt-primary">
        Session expired
      </p>
      <p class="mt-1 text-xs text-mnt-secondary">
        {{
          reauthStalled
            ? 'Signing in again did not restore the session. Retry, or check with whoever runs the identity provider.'
            : 'Your identity provider ended the session. Redirecting you to sign in again…'
        }}
      </p>
      <button
        type="button"
        class="focus-ring mt-4 inline-flex items-center gap-1.5 rounded-lg border border-mnt-default px-3 py-1.5 text-xs font-semibold text-mnt-secondary transition-colors hover:text-mnt-primary"
        @click="startReauth()"
      >
        <LogIn :size="13" aria-hidden="true" />
        Sign in again
      </button>
    </div>
  </div>
</template>

<style scoped>
/* The `bg-mnt-*` utilities are hand-written, so Tailwind's `/80` opacity
   modifier does not apply to them — mix the token here instead. */
.session-scrim {
  background-color: color-mix(in srgb, var(--mnt-bg-primary) 80%, transparent);
}
</style>
