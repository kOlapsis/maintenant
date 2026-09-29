<!--
  Copyright 2026 Benjamin Touchard (kOlapsis)
  SPDX-License-Identifier: Apache-2.0
-->

<script setup lang="ts">
import { ref } from 'vue'
import { Check, Copy } from 'lucide-vue-next'
import TabNav, { type TabNavItem } from './ui/TabNav.vue'
import UiButton from './ui/UiButton.vue'

defineProps<{
  snippets: Record<string, string>
}>()

const activeTab = ref('curl')
const copied = ref(false)

const tabs: TabNavItem[] = [
  { value: 'curl', label: 'curl' },
  { value: 'wget', label: 'wget' },
  { value: 'python', label: 'Python' },
  { value: 'go', label: 'Go' },
  { value: 'bash', label: 'Bash' },
  { value: 'docker_healthcheck', label: 'Docker' },
]

async function copySnippet(code: string) {
  await navigator.clipboard.writeText(code)
  copied.value = true
  setTimeout(() => (copied.value = false), 2000)
}
</script>

<template>
  <div>
    <h3 class="mb-2 text-sm font-semibold" style="color: var(--mnt-text-primary)">Integration Snippets</h3>

    <!-- Tabs -->
    <TabNav v-model="activeTab" :items="tabs" ariaLabel="Snippet language" />

    <!-- Code block -->
    <div class="relative mt-2">
      <pre
        class="overflow-x-auto rounded-lg p-4 font-mono text-sm"
        style="background: var(--mnt-bg-elevated); color: var(--mnt-text-primary)"
      >{{ snippets[activeTab] || '' }}</pre>
      <UiButton
        variant="ghost"
        size="sm"
        :icon="copied ? Check : Copy"
        class="absolute right-2 top-2"
        :class="copied ? 'text-mnt-status-ok' : ''"
        aria-label="Copy snippet"
        @click="copySnippet(snippets[activeTab] || '')"
      >
        {{ copied ? 'Copied!' : 'Copy' }}
      </UiButton>
    </div>
  </div>
</template>
