// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import UiModal from '@/components/ui/UiModal.vue'

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.style.overflow = ''
})

describe('UiModal', () => {
  it('does not render the dialog when closed', () => {
    wrapper = mount(UiModal, { props: { title: 'Confirm', open: false } })
    expect(document.querySelector('[role="dialog"]')).toBeNull()
  })

  it('renders the dialog and moves focus into it when open', async () => {
    wrapper = mount(UiModal, {
      props: { title: 'Confirm', open: true },
      slots: { default: '<button id="inner">OK</button>' },
      attachTo: document.body,
    })
    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    await vi.waitFor(() => {
      expect(dialog?.contains(document.activeElement)).toBe(true)
    })
  })

  it('closes on Escape when dismissible (the default)', async () => {
    wrapper = mount(UiModal, { props: { title: 'Confirm', open: true }, attachTo: document.body })
    document
      .querySelector('[role="dialog"]')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
  })

  it('ignores Escape when not dismissible', async () => {
    wrapper = mount(UiModal, {
      props: { title: 'Confirm', open: true, dismissible: false },
      attachTo: document.body,
    })
    document
      .querySelector('[role="dialog"]')
      ?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('update:open')).toBeUndefined()
  })
})
