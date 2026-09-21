// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

vi.mock('../authGuard', () => ({ probeAuth: vi.fn() }))

class FakeEventSource {
  static instances: FakeEventSource[] = []
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false

  constructor(public url: string) {
    FakeEventSource.instances.push(this)
  }

  addEventListener() {}
  close() {
    this.closed = true
  }

  static get last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]!
  }
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => state === 'hidden' })
  document.dispatchEvent(new Event('visibilitychange'))
}

// Each test gets a fresh module instance; the previous one must be disconnected
// or its document listeners keep reopening connections.
let openBus: typeof import('../sseBus') | null = null

async function loadBus() {
  vi.resetModules()
  openBus = await import('../sseBus')
  return openBus
}

describe('sseBus visibility suspension', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  afterEach(() => {
    openBus?.disconnect()
    openBus = null
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('closes the stream once the page has been hidden past the grace delay', async () => {
    const bus = await loadBus()
    bus.connect()
    FakeEventSource.last.onopen?.()
    expect(bus.connected.value).toBe(true)

    setVisibility('hidden')
    vi.advanceTimersByTime(30_000)
    expect(FakeEventSource.last.closed).toBe(false)

    vi.advanceTimersByTime(31_000)
    expect(FakeEventSource.last.closed).toBe(true)
    expect(bus.connected.value).toBe(false)
    expect(bus.suspended.value).toBe(true)
  })

  it('leaves the stream alone for a short trip to another tab', async () => {
    const bus = await loadBus()
    bus.connect()
    FakeEventSource.last.onopen?.()

    setVisibility('hidden')
    vi.advanceTimersByTime(10_000)
    setVisibility('visible')
    vi.advanceTimersByTime(120_000)

    expect(FakeEventSource.instances).toHaveLength(1)
    expect(FakeEventSource.last.closed).toBe(false)
    expect(bus.suspended.value).toBe(false)
  })

  it('reopens on return and broadcasts sse.reconnected', async () => {
    const bus = await loadBus()
    const onReconnected = vi.fn()
    bus.on('sse.reconnected', onReconnected)
    bus.connect()
    FakeEventSource.last.onopen?.()

    setVisibility('hidden')
    vi.advanceTimersByTime(61_000)
    setVisibility('visible')

    expect(FakeEventSource.instances).toHaveLength(2)
    expect(bus.suspended.value).toBe(false)

    FakeEventSource.last.onopen?.()
    expect(bus.connected.value).toBe(true)
    expect(onReconnected).toHaveBeenCalledTimes(1)
  })

  it('closes immediately on freeze', async () => {
    const bus = await loadBus()
    bus.connect()
    FakeEventSource.last.onopen?.()

    document.dispatchEvent(new Event('freeze'))

    expect(FakeEventSource.last.closed).toBe(true)
    expect(bus.suspended.value).toBe(true)
    expect(bus.connected.value).toBe(false)
  })

  it('clears the backoff so the resumed stream does not wait on a retry timer', async () => {
    const bus = await loadBus()
    bus.connect()
    FakeEventSource.last.onopen?.()
    FakeEventSource.last.onerror?.()
    expect(FakeEventSource.instances).toHaveLength(1)

    setVisibility('hidden')
    document.dispatchEvent(new Event('freeze'))
    expect(FakeEventSource.instances).toHaveLength(1)

    setVisibility('visible')
    expect(FakeEventSource.instances).toHaveLength(2)

    vi.advanceTimersByTime(120_000)
    expect(FakeEventSource.instances).toHaveLength(2)
  })

  it('does not reopen anything once the last consumer disconnected', async () => {
    const bus = await loadBus()
    bus.connect()
    FakeEventSource.last.onopen?.()
    bus.disconnect()
    expect(FakeEventSource.last.closed).toBe(true)

    setVisibility('hidden')
    vi.advanceTimersByTime(61_000)
    setVisibility('visible')

    expect(FakeEventSource.instances).toHaveLength(1)
  })
})
