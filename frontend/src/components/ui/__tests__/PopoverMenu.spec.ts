// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import PopoverMenu from '@/components/ui/PopoverMenu.vue'

function mountMenu(open = false) {
  return mount(PopoverMenu, {
    props: { ariaLabel: 'Demo menu', open },
    slots: {
      trigger: '<button type="button" class="trigger">Open</button>',
      default: `
        <button type="button" role="menuitem" class="item-a">A</button>
        <button type="button" role="menuitem" class="item-b">B</button>
      `,
    },
    attachTo: document.body,
  })
}

describe('PopoverMenu', () => {
  it('renders the panel only when open', () => {
    const closed = mountMenu(false)
    expect(closed.find('[role="menu"]').exists()).toBe(false)

    const open = mountMenu(true)
    expect(open.find('[role="menu"]').exists()).toBe(true)
    open.unmount()
    closed.unmount()
  })

  it('closes on Escape', async () => {
    const wrapper = mountMenu(true)
    await wrapper.trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
    wrapper.unmount()
  })

  it('closes on an outside pointerdown', async () => {
    const wrapper = mountMenu(true)
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
    wrapper.unmount()
  })

  it('moves focus between items with ArrowDown/ArrowUp', async () => {
    const wrapper = mountMenu(true)
    const items = wrapper.findAll('[role="menuitem"]')
    ;(items[0]!.element as HTMLElement).focus()
    await wrapper.trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(items[1]!.element)

    await wrapper.trigger('keydown', { key: 'ArrowUp' })
    expect(document.activeElement).toBe(items[0]!.element)
    wrapper.unmount()
  })
})
