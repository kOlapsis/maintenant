// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { showToast, useToast } from '../useToast'

beforeEach(() => {
  useToast().toasts.value = []
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('showToast', () => {
  it('adds a toast', () => {
    showToast('hello', 'info')
    expect(useToast().toasts.value).toHaveLength(1)
    expect(useToast().toasts.value[0]?.message).toBe('hello')
  })

  it('carries an optional title alongside the message', () => {
    showToast('body', 'warning', 5000, { title: 'Demo mode' })
    expect(useToast().toasts.value[0]?.title).toBe('Demo mode')
    expect(useToast().toasts.value[0]?.message).toBe('body')
  })

  it('does not stack a second toast sharing the same dedupeKey', () => {
    showToast('first', 'warning', 5000, { dedupeKey: 'demo-mode' })
    showToast('second', 'warning', 5000, { dedupeKey: 'demo-mode' })
    expect(useToast().toasts.value).toHaveLength(1)
    expect(useToast().toasts.value[0]?.message).toBe('first')
  })

  it('lets a different dedupeKey through', () => {
    showToast('first', 'warning', 5000, { dedupeKey: 'demo-mode' })
    showToast('second', 'warning', 5000, { dedupeKey: 'storage-outage' })
    expect(useToast().toasts.value).toHaveLength(2)
  })

  it('allows a new toast once the deduped one has expired', () => {
    showToast('first', 'warning', 1000, { dedupeKey: 'demo-mode' })
    vi.advanceTimersByTime(1000)
    showToast('second', 'warning', 1000, { dedupeKey: 'demo-mode' })
    expect(useToast().toasts.value).toHaveLength(1)
    expect(useToast().toasts.value[0]?.message).toBe('second')
  })
})
