// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FileInput from '@/components/ui/FileInput.vue'

function makeFile(name = 'logo.png') {
  return new File(['x'], name, { type: 'image/png' })
}

describe('FileInput', () => {
  it('shows no filename and no clear button until a file is picked', () => {
    const wrapper = mount(FileInput, { props: { file: null } })
    expect(wrapper.text()).not.toContain('.png')
    expect(wrapper.findAll('button')).toHaveLength(1)
  })

  it('emits update:file with the picked file', async () => {
    const wrapper = mount(FileInput, { props: { file: null } })
    const file = makeFile()
    const input = wrapper.find('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file] })
    await input.trigger('change')
    expect(wrapper.emitted('update:file')?.[0]).toEqual([file])
  })

  it('shows the filename and a clear button once a file is set, and clears on click', async () => {
    const wrapper = mount(FileInput, { props: { file: makeFile('hero.png') } })
    expect(wrapper.text()).toContain('hero.png')
    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    await buttons[1]!.trigger('click')
    expect(wrapper.emitted('update:file')?.[0]).toEqual([null])
  })
})
