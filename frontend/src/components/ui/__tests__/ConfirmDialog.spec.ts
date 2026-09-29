// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'
import type { ConfirmState } from '@/composables/useConfirm'

function makeState(overrides: Partial<ConfirmState> = {}): ConfirmState {
  return {
    title: 'Delete item',
    message: 'This cannot be undone.',
    resolve: vi.fn(),
    ...overrides,
  }
}

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.style.overflow = ''
})

describe('ConfirmDialog', () => {
  it('renders nothing when state is null', () => {
    wrapper = mount(ConfirmDialog, { props: { state: null }, attachTo: document.body })
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('renders the title and message when state is set', () => {
    const state = makeState()
    wrapper = mount(ConfirmDialog, { props: { state }, attachTo: document.body })
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Delete item')
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('This cannot be undone.')
  })

  it('resolves false on cancel and true on confirm', () => {
    const state = makeState()
    wrapper = mount(ConfirmDialog, { props: { state }, attachTo: document.body })
    const buttons = document.querySelectorAll('[role="dialog"] button')
    const cancelButton = Array.from(buttons).find((b) => b.textContent?.trim() === 'Cancel')
    cancelButton?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(state.resolve).toHaveBeenCalledWith(false)
  })
})
