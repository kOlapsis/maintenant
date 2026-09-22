// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { useVisibleInterval } from '../useVisibleInterval'

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => state === 'hidden' })
  document.dispatchEvent(new Event('visibilitychange'))
}

function mountWith(fn: () => void, intervalMs: number) {
  return mount(
    defineComponent({
      setup() {
        useVisibleInterval(fn, intervalMs)
        return () => null
      },
    }),
  )
}

describe('useVisibleInterval', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' })
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => false })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('runs on every tick while the page is visible', () => {
    const fn = vi.fn()
    mountWith(fn, 1_000)
    vi.advanceTimersByTime(3_000)
    expect(fn).toHaveBeenCalledTimes(3)
  })

  it('skips every tick while the page is hidden and catches up once on return', () => {
    const fn = vi.fn()
    mountWith(fn, 1_000)

    setVisibility('hidden')
    vi.advanceTimersByTime(5_000)
    expect(fn).not.toHaveBeenCalled()

    setVisibility('visible')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('does not catch up when no tick was missed', () => {
    const fn = vi.fn()
    mountWith(fn, 10_000)

    setVisibility('hidden')
    vi.advanceTimersByTime(1_000)
    setVisibility('visible')

    expect(fn).not.toHaveBeenCalled()
  })

  it('stops the timer and the listener on unmount', () => {
    const fn = vi.fn()
    const wrapper = mountWith(fn, 1_000)
    wrapper.unmount()

    vi.advanceTimersByTime(5_000)
    setVisibility('hidden')
    setVisibility('visible')

    expect(fn).not.toHaveBeenCalled()
  })
})
