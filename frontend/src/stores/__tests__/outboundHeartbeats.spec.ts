// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const listOutboundHeartbeats = vi.hoisted(() => vi.fn())
const createOutboundHeartbeat = vi.hoisted(() => vi.fn())
const updateOutboundHeartbeat = vi.hoisted(() => vi.fn())
const deleteOutboundHeartbeat = vi.hoisted(() => vi.fn())
const sendOutboundHeartbeat = vi.hoisted(() => vi.fn())

vi.mock('@/services/outboundHeartbeatApi', () => ({
  listOutboundHeartbeats,
  createOutboundHeartbeat,
  updateOutboundHeartbeat,
  deleteOutboundHeartbeat,
  sendOutboundHeartbeat,
}))

import { useOutboundHeartbeatsStore } from '../outboundHeartbeats'
import type { OutboundHeartbeat } from '@/services/outboundHeartbeatApi'

function target(overrides: Partial<OutboundHeartbeat> = {}): OutboundHeartbeat {
  return {
    id: 't1',
    name: 'DR site',
    url: 'https://dr.example.com/ping/abc',
    interval_seconds: 300,
    enabled: true,
    last_sent_at: null,
    last_status_code: null,
    last_error: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  listOutboundHeartbeats.mockReset()
  createOutboundHeartbeat.mockReset()
  updateOutboundHeartbeat.mockReset()
  deleteOutboundHeartbeat.mockReset()
  sendOutboundHeartbeat.mockReset()
})

describe('outbound heartbeats store', () => {
  it('loads the list', async () => {
    listOutboundHeartbeats.mockResolvedValue({ outbound_heartbeats: [target()] })
    const store = useOutboundHeartbeatsStore()

    await store.fetchOutboundHeartbeats()

    expect(store.outboundHeartbeats).toHaveLength(1)
    expect(store.loading).toBe(false)
    expect(store.error).toBeNull()
  })

  it('surfaces a fetch failure without throwing', async () => {
    listOutboundHeartbeats.mockRejectedValue(new Error('boom'))
    const store = useOutboundHeartbeatsStore()

    await store.fetchOutboundHeartbeats()

    expect(store.error).toBe('boom')
    expect(store.outboundHeartbeats).toEqual([])
  })

  it('appends a created target', async () => {
    createOutboundHeartbeat.mockResolvedValue(target())
    const store = useOutboundHeartbeatsStore()

    await store.create({ name: 'DR site', url: 'https://dr.example.com/ping/abc', interval_seconds: 300, enabled: true })

    expect(store.outboundHeartbeats).toHaveLength(1)
    expect(store.outboundHeartbeats[0]?.id).toBe('t1')
  })

  it('replaces the row in place on update, including a toggle', async () => {
    listOutboundHeartbeats.mockResolvedValue({ outbound_heartbeats: [target()] })
    const store = useOutboundHeartbeatsStore()
    await store.fetchOutboundHeartbeats()

    updateOutboundHeartbeat.mockResolvedValue(target({ enabled: false }))
    await store.update('t1', { name: 'DR site', url: 'https://dr.example.com/ping/abc', interval_seconds: 300, enabled: false })

    expect(store.outboundHeartbeats).toHaveLength(1)
    expect(store.outboundHeartbeats[0]?.enabled).toBe(false)
  })

  it('removes a target on delete', async () => {
    listOutboundHeartbeats.mockResolvedValue({ outbound_heartbeats: [target()] })
    const store = useOutboundHeartbeatsStore()
    await store.fetchOutboundHeartbeats()

    deleteOutboundHeartbeat.mockResolvedValue(undefined)
    await store.remove('t1')

    expect(store.outboundHeartbeats).toEqual([])
  })

  it('reflects a manual send result on the row', async () => {
    listOutboundHeartbeats.mockResolvedValue({ outbound_heartbeats: [target()] })
    const store = useOutboundHeartbeatsStore()
    await store.fetchOutboundHeartbeats()

    sendOutboundHeartbeat.mockResolvedValue(
      target({ last_sent_at: '2026-01-02T00:00:00Z', last_status_code: 200 }),
    )
    await store.sendNow('t1')

    expect(store.outboundHeartbeats[0]?.last_status_code).toBe(200)
    expect(store.outboundHeartbeats[0]?.last_sent_at).toBe('2026-01-02T00:00:00Z')
  })
})
