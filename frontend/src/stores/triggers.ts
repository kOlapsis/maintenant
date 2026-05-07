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
import {
  listTriggers,
  createTrigger,
  updateTrigger,
  deleteTrigger,
} from '@/services/triggersApi'
import type { AlertTrigger, TriggerRequest } from '@/types/triggers'
import { sseBus } from '@/services/sseBus'

export const useTriggersStore = defineStore('triggers', () => {
  const triggers = ref<AlertTrigger[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function fetchTriggers() {
    loading.value = true
    error.value = null
    try {
      const res = await listTriggers()
      triggers.value = res.triggers
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch triggers'
    } finally {
      loading.value = false
    }
  }

  async function create(req: TriggerRequest): Promise<AlertTrigger> {
    const t = await createTrigger(req)
    triggers.value = [...triggers.value, t]
    return t
  }

  async function update(id: number, req: TriggerRequest): Promise<AlertTrigger> {
    const t = await updateTrigger(id, req)
    const idx = triggers.value.findIndex((x) => x.id === id)
    if (idx >= 0) triggers.value[idx] = t
    return t
  }

  async function remove(id: number): Promise<void> {
    await deleteTrigger(id)
    triggers.value = triggers.value.filter((t) => t.id !== id)
  }

  function triggersForChannel(channelId: number): AlertTrigger[] {
    return triggers.value.filter((t) => t.enabled && t.channel_ids.includes(channelId))
  }

  function onTriggerCreated(e: MessageEvent) {
    let t: AlertTrigger
    try {
      t = JSON.parse(e.data)
    } catch {
      return
    }
    if (!triggers.value.find((x) => x.id === t.id)) {
      triggers.value = [...triggers.value, t]
    }
  }

  function onTriggerUpdated(e: MessageEvent) {
    let t: AlertTrigger
    try {
      t = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = triggers.value.findIndex((x) => x.id === t.id)
    if (idx >= 0) triggers.value[idx] = t
  }

  function onTriggerDeleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    triggers.value = triggers.value.filter((t) => t.id !== data.id)
  }

  function connectSSE() {
    sseBus.on('trigger.created', onTriggerCreated)
    sseBus.on('trigger.updated', onTriggerUpdated)
    sseBus.on('trigger.deleted', onTriggerDeleted)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('trigger.created', onTriggerCreated)
    sseBus.off('trigger.updated', onTriggerUpdated)
    sseBus.off('trigger.deleted', onTriggerDeleted)
  }

  return {
    triggers,
    loading,
    error,
    fetchTriggers,
    create,
    update,
    remove,
    triggersForChannel,
    connectSSE,
    disconnectSSE,
  }
})
