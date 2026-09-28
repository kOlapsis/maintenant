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
import { ref, computed, onMounted } from 'vue'
import { useNamespacesStore } from '@/stores/namespaces'
import { ChevronDown, Check, Layers } from 'lucide-vue-next'
import PopoverMenu from '@/components/ui/PopoverMenu.vue'
import UiButton from '@/components/ui/UiButton.vue'

const store = useNamespacesStore()

const open = ref(false)

const label = computed(() => {
  const count = store.selectedNamespaces.length
  if (count === 0) return 'All namespaces'
  if (count === 1) return store.selectedNamespaces[0] ?? 'All namespaces'
  return `${count} namespaces`
})

onMounted(() => {
  store.fetchNamespacesList()
})
</script>

<template>
  <PopoverMenu v-model:open="open" ariaLabel="Namespaces" panel-class="min-w-52">
    <template #trigger="{ toggle }">
      <UiButton
        variant="secondary"
        class="bg-mnt-surface text-mnt-secondary hover:text-mnt-primary"
        @click="toggle"
      >
        <Layers :size="14" class="text-mnt-muted flex-shrink-0" />
        <span class="truncate max-w-40">{{ label }}</span>
        <ChevronDown
          :size="14"
          :class="['text-mnt-muted flex-shrink-0 transition-transform', open ? 'rotate-180' : '']"
        />
      </UiButton>
    </template>

    <!-- All namespaces option -->
    <UiButton
      variant="ghost"
      block
      align="between"
      role="menuitemcheckbox"
      :aria-checked="store.selectedNamespaces.length === 0"
      class="rounded-none px-4 py-2.5 text-sm text-mnt-secondary hover:text-mnt-primary"
      @click="store.selectAll()"
    >
      <span>All namespaces</span>
      <Check
        v-if="store.selectedNamespaces.length === 0"
        :size="14"
        class="text-mnt-green-400 flex-shrink-0"
      />
    </UiButton>

    <!-- Divider -->
    <div v-if="store.namespaces.length > 0" class="border-t border-mnt-default" />

    <!-- Individual namespaces -->
    <div class="max-h-64 overflow-y-auto">
      <UiButton
        v-for="ns in store.namespaces"
        :key="ns"
        variant="ghost"
        block
        align="between"
        role="menuitemcheckbox"
        :aria-checked="store.selectedNamespaces.includes(ns)"
        class="rounded-none px-4 py-2.5 text-sm"
        :class="store.selectedNamespaces.includes(ns) ? 'text-mnt-primary' : 'text-mnt-muted'"
        @click="store.toggleNamespace(ns)"
      >
        <span class="font-mono">{{ ns }}</span>
        <Check
          v-if="store.selectedNamespaces.includes(ns)"
          :size="14"
          class="text-mnt-green-400 flex-shrink-0"
        />
      </UiButton>
    </div>

    <!-- Empty state -->
    <div v-if="store.namespaces.length === 0" class="px-4 py-3 text-xs text-mnt-muted text-center">
      No namespaces found
    </div>
  </PopoverMenu>
</template>
