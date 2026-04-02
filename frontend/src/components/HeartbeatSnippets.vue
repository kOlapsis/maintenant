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

defineProps<{
  snippets: Record<string, string>
}>()

const activeTab = ref('curl')
const copied = ref(false)

const tabs = [
  { key: 'curl', label: 'curl' },
  { key: 'wget', label: 'wget' },
  { key: 'python', label: 'Python' },
  { key: 'go', label: 'Go' },
  { key: 'bash', label: 'Bash' },
  { key: 'docker_healthcheck', label: 'Docker' },
]

async function copySnippet(code: string) {
  await navigator.clipboard.writeText(code)
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}
</script>

<template>
  <div>
    <h3 class="mb-2 text-sm font-semibold" style="color: var(--pb-text-primary)">Integration Snippets</h3>

    <!-- Tabs -->
    <div class="mb-2 flex gap-1 border-b" style="border-color: var(--pb-border-default)">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="rounded-t px-3 py-1.5 text-xs font-medium transition-colors"
        :style="{
          borderBottom: activeTab === tab.key ? '2px solid var(--pb-accent)' : '2px solid transparent',
          color: activeTab === tab.key ? 'var(--pb-accent)' : 'var(--pb-text-muted)',
        }"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Code block -->
    <div class="relative">
      <pre
        class="overflow-x-auto rounded-lg p-4 font-mono text-sm"
        style="background: var(--pb-bg-elevated); color: var(--pb-text-primary)"
      >{{ snippets[activeTab] || '' }}</pre>
      <button
        class="absolute right-2 top-2 flex items-center gap-1 rounded px-2 py-1 text-xs transition-all"
        :style="{
          background: copied ? 'var(--pb-status-ok-bg)' : 'var(--pb-bg-hover)',
          color: copied ? 'var(--pb-status-ok)' : 'var(--pb-text-muted)',
        }"
        aria-label="Copy snippet"
        @click="copySnippet(snippets[activeTab] || '')"
      >
        <!-- Checkmark icon when copied -->
        <svg
          v-if="copied"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          class="transition-all"
        >
          <path d="M3 7.5L5.5 10L11 4" />
        </svg>
        <!-- Copy icon when not copied -->
        <svg
          v-else
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <rect x="4.5" y="4.5" width="7" height="7" rx="1" />
          <path d="M9.5 4.5V3a1 1 0 0 0-1-1H3a1 1 0 0 0-1 1v5.5a1 1 0 0 0 1 1h1.5" />
        </svg>
        {{ copied ? 'Copied!' : 'Copy' }}
      </button>
    </div>
  </div>
</template>
