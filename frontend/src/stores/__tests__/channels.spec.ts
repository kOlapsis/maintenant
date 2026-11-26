// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useChannelsStore } from '@/stores/channels'

const listChannels = vi.hoisted(() => vi.fn())
vi.mock('@/services/alertApi', () => ({ listChannels }))

const bus = vi.hoisted(() => ({
  handlers: new Map<string, (e: MessageEvent) => void>(),
  connect: vi.fn(),
  disconnect: vi.fn(),
}))
vi.mock('@/services/sseBus', () => ({
  sseBus: {
    on: (type: string, fn: (e: MessageEvent) => void) => bus.handlers.set(type, fn),
    off: (type: string) => bus.handlers.delete(type),
    connect: bus.connect,
    disconnect: bus.disconnect,
  },
}))

describe('channels store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    listChannels.mockReset()
    bus.handlers.clear()
    bus.connect.mockReset()
    bus.disconnect.mockReset()
  })

  it('keeps the reason when the channels could not be loaded', async () => {
    listChannels.mockRejectedValue(new Error('HTTP 503'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const store = useChannelsStore()

    await store.fetchChannels()
    expect(store.channelsError).toBe('HTTP 503')

    listChannels.mockResolvedValue({ channels: [] })
    await store.fetchChannels()
    expect(store.channelsError).toBeNull()
  })

  it('releases the live stream it asked for', () => {
    const store = useChannelsStore()

    store.connectSSE()
    store.disconnectSSE()

    expect(bus.connect).toHaveBeenCalledTimes(1)
    expect(bus.disconnect).toHaveBeenCalledTimes(1)
  })

  it('lists a created channel once', () => {
    const store = useChannelsStore()
    store.connectSSE()
    store.channels = [{ id: 'c1', name: 'ops' }] as never
    const created = (name: string) => new MessageEvent('channel.created', { data: JSON.stringify({ id: 'c1', name }) })

    bus.handlers.get('channel.created')!(created('ops renamed'))

    expect(store.channels.map((c) => [c.id, c.name])).toEqual([['c1', 'ops renamed']])
  })
})
