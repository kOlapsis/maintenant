// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { fetchNamespaces } from '@/services/kubernetesApi'

const STORAGE_KEY = 'maintenant:k8s:namespaces'

function loadFromStorage(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (Array.isArray(parsed) && parsed.every((v) => typeof v === 'string')) {
      return parsed as string[]
    }
  } catch { /* ignore */ }
  return []
}

function saveToStorage(selected: string[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(selected))
  } catch { /* ignore */ }
}

export const useNamespacesStore = defineStore('namespaces', () => {
  const namespaces = ref<string[]>([])
  const selectedNamespaces = ref<string[]>(loadFromStorage())

  const namespacesParam = computed(() => {
    if (selectedNamespaces.value.length === 0) return ''
    return selectedNamespaces.value.join(',')
  })

  async function fetchNamespacesList() {
    try {
      const resp = await fetchNamespaces()
      namespaces.value = resp.namespaces
    } catch { /* ignore */ }
  }

  function toggleNamespace(ns: string) {
    const idx = selectedNamespaces.value.indexOf(ns)
    if (idx >= 0) {
      selectedNamespaces.value = selectedNamespaces.value.filter((n) => n !== ns)
    } else {
      selectedNamespaces.value = [...selectedNamespaces.value, ns]
    }
    saveToStorage(selectedNamespaces.value)
  }

  function selectAll() {
    selectedNamespaces.value = []
    saveToStorage(selectedNamespaces.value)
  }

  return {
    namespaces,
    selectedNamespaces,
    namespacesParam,
    fetchNamespacesList,
    toggleNamespace,
    selectAll,
  }
})
