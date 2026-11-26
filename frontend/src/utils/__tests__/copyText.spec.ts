// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { copyText } from '../copyText'

const showToast = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useToast', () => ({ showToast }))

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, 'clipboard', { value, configurable: true })
}

describe('copyText', () => {
  beforeEach(() => {
    showToast.mockReset()
  })

  afterEach(() => {
    setClipboard(undefined)
    Reflect.deleteProperty(document, 'execCommand')
  })

  it('uses the clipboard when the browser offers it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    setClipboard({ writeText })

    expect(await copyText('token')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('token')
  })

  it('still copies on a page served over plain HTTP', async () => {
    setClipboard(undefined)
    let copiedValue = ''
    document.execCommand = vi.fn(() => {
      copiedValue = document.querySelector('textarea')!.value
      return true
    })

    expect(await copyText('token')).toBe(true)
    expect(copiedValue).toBe('token')
    expect(document.querySelector('textarea')).toBeNull()
    expect(showToast).not.toHaveBeenCalled()
  })

  it('says so when nothing could be copied', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) })

    expect(await copyText('token')).toBe(false)
    expect(showToast).toHaveBeenCalledWith('Could not copy. Select the text and copy it by hand.', 'warning')
  })
})
