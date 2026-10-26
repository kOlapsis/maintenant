// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, afterEach } from 'vitest'
import { mount, flushPromises, type VueWrapper, type DOMWrapper } from '@vue/test-utils'
import HeartbeatForm from '@/components/HeartbeatForm.vue'
import type { CreateHeartbeatInput } from '@/services/heartbeatApi'

// 048-heartbeat-edit: US1 scenarios 1, 2 and 5, US3 scenarios 1 and 3, FR-001, FR-007, contract api.md.

const SUBMIT = 'Save'
const mounted: VueWrapper[] = []

afterEach(() => {
  while (mounted.length) mounted.pop()!.unmount()
  document.body.innerHTML = ''
})

function mountForm(initial: CreateHeartbeatInput) {
  // Attached, so a click on the submit button submits the form as in a browser, native constraints included.
  const wrapper = mount(HeartbeatForm, { props: { initial, submitLabel: SUBMIT }, attachTo: document.body })
  mounted.push(wrapper)
  return wrapper
}

function fieldByLabel(wrapper: VueWrapper, label: RegExp): DOMWrapper<HTMLInputElement> {
  const tag = wrapper.findAll('label').find((l) => label.test(l.text()))
  expect(tag, `a field labelled ${label}`).toBeDefined()
  const id = tag!.attributes('for')
  const input = id ? wrapper.find<HTMLInputElement>(`[id="${id}"]`) : tag!.find<HTMLInputElement>('input')
  expect(input.exists(), `the label ${label} names its input`).toBe(true)
  return input
}

const nameField = (w: VueWrapper) => fieldByLabel(w, /name/i)
const graceField = (w: VueWrapper) => fieldByLabel(w, /grace/i)

function selectedIntervalLabel(wrapper: VueWrapper): string {
  const pressed = wrapper.find('[aria-pressed="true"]')
  if (pressed.exists()) return pressed.text().trim()
  const select = wrapper.find<HTMLSelectElement>('select')
  if (select.exists()) return select.element.selectedOptions[0]?.text.trim() ?? ''
  return ''
}

async function chooseInterval(wrapper: VueWrapper, label: string) {
  const button = wrapper.findAll('button').find((b) => b.text().trim() === label)
  if (button) {
    await button.trigger('click')
    return
  }
  const select = wrapper.find<HTMLSelectElement>('select')
  const option = select.exists() ? Array.from(select.element.options).find((o) => o.text.trim() === label) : undefined
  expect(option, `the interval ${label} can be chosen`).toBeDefined()
  await select.setValue(option!.value)
}

async function submit(wrapper: VueWrapper) {
  const button = wrapper.findAll('button').find((b) => b.text().trim() === SUBMIT)
  expect(button, `a "${SUBMIT}" button`).toBeDefined()
  ;(button!.element as HTMLButtonElement).click()
  await flushPromises()
}

function occurrences(text: string, pattern: RegExp): number {
  return (text.match(new RegExp(pattern.source, 'gi')) ?? []).length
}

// A refusal is a message on the page that says what to fix, or the browser's own message on the field,
// which carries the limit when the field declares it.
async function expectRefused(
  wrapper: VueWrapper,
  input: DOMWrapper<HTMLInputElement>,
  baseline: string,
  opts: { why: string; mentions: RegExp; native: (el: HTMLInputElement) => boolean },
) {
  await submit(wrapper)
  expect(wrapper.emitted('submit'), `${opts.why}: nothing is sent`).toBeUndefined()
  const onPage = occurrences(wrapper.text(), opts.mentions) > occurrences(baseline, opts.mentions)
  const byBrowser = !input.element.checkValidity() && opts.native(input.element)
  expect(onPage || byBrowser, `${opts.why}: a message says what to fix (page: "${wrapper.text()}")`).toBe(true)
}

const current = [
  { name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300, shown: /^1h$/ },
  { name: 'weekly report', interval_seconds: 604800, grace_seconds: 0, shown: /^7d$/ },
  { name: 'every two hours', interval_seconds: 7200, grace_seconds: 900, shown: /2\s*h|2 hours/i },
]

describe('HeartbeatForm', () => {
  it.each(current)('US1 sc.1: is prefilled with the current values of "$name"', ({ shown, ...initial }) => {
    const wrapper = mountForm(initial)

    expect(nameField(wrapper).element.value).toBe(initial.name)
    expect(graceField(wrapper).element.value).toBe(String(initial.grace_seconds))
    expect(selectedIntervalLabel(wrapper), 'the current interval is the selected one').toMatch(shown)
  })

  it.each(current)('US1 sc.1, FR-001: saved untouched, "$name" keeps its three values', async ({ shown: _, ...initial }) => {
    const wrapper = mountForm(initial)

    await submit(wrapper)

    expect(wrapper.emitted('submit')).toEqual([[initial]])
  })

  it('US1 sc.2, contract api.md: sends the three edited values, as numbers', async () => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 86400, grace_seconds: 600 })

    await nameField(wrapper).setValue('nightly db backup')
    await chooseInterval(wrapper, '15m')
    await graceField(wrapper).setValue('120')
    await submit(wrapper)

    expect(wrapper.emitted('submit')).toEqual([[{ name: 'nightly db backup', interval_seconds: 900, grace_seconds: 120 }]])
  })

  it('US1 sc.2: a grace-only change still sends the three values', async () => {
    const wrapper = mountForm({ name: 'hourly sync', interval_seconds: 3600, grace_seconds: 60 })

    await graceField(wrapper).setValue('0')
    await submit(wrapper)

    expect(wrapper.emitted('submit')).toEqual([[{ name: 'hourly sync', interval_seconds: 3600, grace_seconds: 0 }]])
  })

  it('US1 sc.5: cancel sends nothing', async () => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })

    await nameField(wrapper).setValue('renamed')
    const cancel = wrapper.findAll('button').find((b) => /cancel/i.test(b.text()))
    expect(cancel, 'a cancel button').toBeDefined()
    await cancel!.trigger('click')

    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('submit')).toBeUndefined()
  })

  it('US3 sc.1, FR-007: refuses an empty name with a message on the name', async () => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })
    const baseline = wrapper.text()
    const input = nameField(wrapper)

    await input.setValue('')

    await expectRefused(wrapper, input, baseline, {
      why: 'empty name',
      mentions: /name|required/,
      native: (el) => el.validity.valueMissing,
    })
  })

  it('US3 sc.1, FR-007: a name stops at 255 characters', async () => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })
    const input = nameField(wrapper)

    await input.setValue('n'.repeat(255))
    await submit(wrapper)
    expect(wrapper.emitted('submit'), '255 characters are accepted').toEqual([[{ name: 'n'.repeat(255), interval_seconds: 3600, grace_seconds: 300 }]])

    if (input.element.maxLength > 0) {
      expect(input.element.maxLength, 'the field cannot take more than 255 characters').toBeLessThanOrEqual(255)
      return
    }
    const fresh = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })
    const baseline = fresh.text()
    const freshInput = nameField(fresh)
    await freshInput.setValue('n'.repeat(256))
    await expectRefused(fresh, freshInput, baseline, {
      why: 'name of 256 characters',
      mentions: /name|255/,
      native: () => false,
    })
  })

  it.each([
    { what: 'above the current interval', interval: null, grace: '3601', limit: 3600, human: '1\\s*h' },
    { what: 'above an interval chosen in the form', interval: '5m', grace: '301', limit: 300, human: '5\\s*m' },
  ])('US3 sc.3, FR-007: refuses a grace $what, with the limit', async ({ interval, grace, limit, human }) => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })
    const baseline = wrapper.text()
    if (interval) await chooseInterval(wrapper, interval)
    const input = graceField(wrapper)

    await input.setValue(grace)

    await expectRefused(wrapper, input, baseline, {
      why: `grace ${grace} over ${limit}`,
      mentions: new RegExp(`\\b${limit}\\b|${human}`),
      native: (el) => el.validity.rangeOverflow && Number(el.max) === limit,
    })
  })

  it('US3 sc.3, FR-007: refuses a negative grace', async () => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })
    const baseline = wrapper.text()
    const input = graceField(wrapper)

    await input.setValue('-1')

    await expectRefused(wrapper, input, baseline, {
      why: 'negative grace',
      mentions: /\b0\b|negative/,
      native: (el) => el.validity.rangeUnderflow && Number(el.min) === 0,
    })
  })

  it.each([
    { what: 'a grace equal to the interval', interval: '5m', grace: '300', sent: { interval_seconds: 300, grace_seconds: 300 } },
    { what: 'no grace at all', interval: '12h', grace: '0', sent: { interval_seconds: 43200, grace_seconds: 0 } },
  ])('FR-007: accepts $what', async ({ interval, grace, sent }) => {
    const wrapper = mountForm({ name: 'nightly backup', interval_seconds: 3600, grace_seconds: 300 })

    await chooseInterval(wrapper, interval)
    await graceField(wrapper).setValue(grace)
    await submit(wrapper)

    expect(wrapper.emitted('submit')).toEqual([[{ name: 'nightly backup', ...sent }]])
  })
})
