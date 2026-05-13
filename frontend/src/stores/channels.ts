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
import { listChannels, type NotificationChannel } from '@/services/alertApi'
import { sseBus } from '@/services/sseBus'

export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<NotificationChannel[]>([])
  const channelsLoading = ref(false)

  async function fetchChannels() {
    channelsLoading.value = true
    try {
      const res = await listChannels()
      channels.value = res.channels
    } catch (e) {
      console.error('Failed to fetch channels:', e)
    } finally {
      channelsLoading.value = false
    }
  }

  function onChannelCreated(e: MessageEvent) {
    let channel: NotificationChannel
    try {
      channel = JSON.parse(e.data)
    } catch {
      return
    }
    channels.value = [...channels.value, channel]
  }

  function onChannelUpdated(e: MessageEvent) {
    let channel: NotificationChannel
    try {
      channel = JSON.parse(e.data)
    } catch {
      return
    }
    const idx = channels.value.findIndex((c) => c.id === channel.id)
    if (idx >= 0) channels.value[idx] = channel
  }

  function onChannelDeleted(e: MessageEvent) {
    let data
    try {
      data = JSON.parse(e.data)
    } catch {
      return
    }
    channels.value = channels.value.filter((c) => c.id !== data.id)
  }

  function onReconnected() {
    fetchChannels()
  }

  function connectSSE() {
    sseBus.on('channel.created', onChannelCreated)
    sseBus.on('channel.updated', onChannelUpdated)
    sseBus.on('channel.deleted', onChannelDeleted)
    sseBus.on('sse.reconnected', onReconnected)
    sseBus.connect()
  }

  function disconnectSSE() {
    sseBus.off('channel.created', onChannelCreated)
    sseBus.off('channel.updated', onChannelUpdated)
    sseBus.off('channel.deleted', onChannelDeleted)
    sseBus.off('sse.reconnected', onReconnected)
  }

  return {
    channels,
    channelsLoading,
    fetchChannels,
    connectSSE,
    disconnectSSE,
  }
})
