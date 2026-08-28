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
import { ref } from 'vue'
import { sseBus } from '@/services/sseBus'

export type StorageEngine = 'sqlite' | 'postgres'

export interface StorageHealth {
  engine: StorageEngine
  connected: boolean
  peers: number
}

/**
 * Tracks whether the database backing this instance answers, so the interface
 * can say so instead of rendering empty screens (FR-023).
 *
 * `connected` starts true on purpose: an install on the default local file is
 * never disconnected, and a banner flashing on every page load would be noise.
 * The state only moves when the server says it moved.
 */
export const useStorageStore = defineStore('storage', () => {
  const engine = ref<StorageEngine>('sqlite')
  const connected = ref(true)
  const peers = ref(0)

  /** Reads the initial state from the health diagnostic. */
  async function fetchStatus() {
    try {
      const res = await fetch('/api/v1/health')
      if (!res.ok) return
      const data = (await res.json()) as { storage?: StorageHealth }
      if (!data.storage) return
      engine.value = data.storage.engine
      connected.value = data.storage.connected
      peers.value = data.storage.peers
    } catch {
      // The health endpoint being unreachable is a different problem, and one
      // the SSE connection state already surfaces.
    }
  }

  function onAvailabilityChanged(e: MessageEvent) {
    try {
      const data = JSON.parse(e.data) as { engine?: StorageEngine; connected?: boolean }
      if (data.engine) engine.value = data.engine
      if (typeof data.connected === 'boolean') connected.value = data.connected
    } catch {
      /* ignore */
    }
  }

  function startListening() {
    sseBus.on('storage.availability_changed', onAvailabilityChanged)
  }

  function stopListening() {
    sseBus.off('storage.availability_changed', onAvailabilityChanged)
  }

  return {
    engine,
    connected,
    peers,
    fetchStatus,
    startListening,
    stopListening,
  }
})
