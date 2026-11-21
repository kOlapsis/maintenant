// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UpdateBadge from '@/components/UpdateBadge.vue'
import type { ImageUpdate } from '@/services/updateApi'

function update(extra: Partial<ImageUpdate> = {}): ImageUpdate {
  return {
    id: 'upd-1',
    container_id: 'ctr-1',
    container_name: 'db',
    image: 'postgres:18.7',
    current_tag: '18.7',
    current_digest: '',
    latest_tag: '18.8',
    latest_digest: '',
    update_type: 'minor',
    published_at: null,
    changelog_url: '',
    changelog_summary: '',
    has_breaking_changes: false,
    risk_score: 50,
    status: 'available',
    detected_at: '2026-10-13T10:00:00Z',
    ...extra,
  }
}

function style(extra: Partial<ImageUpdate>): string {
  return mount(UpdateBadge, { props: { update: update(extra) } }).attributes('style') ?? ''
}

const down = 'var(--mnt-status-down)'
const warn = 'var(--mnt-status-warn)'
const muted = 'var(--mnt-text-muted)'

describe('UpdateBadge: color follows the urgency group', () => {
  it.each<[number, string]>([
    [0, muted],
    [30, muted],
    [31, warn],
    [80, warn],
    [81, down],
    [100, down],
  ])('score %i uses %s', (risk_score, color) => {
    for (const update_type of ['major', 'minor', 'patch', 'digest_only']) {
      expect(style({ risk_score, update_type })).toContain(`color: ${color}`)
    }
  })

  it('does not paint a major update red at score 20', () => {
    const s = style({ update_type: 'major', risk_score: 20 })
    expect(s).toContain(`color: ${muted}`)
    expect(s).not.toContain('status-down')
  })

  it('keeps the pinned badge muted whatever the score', () => {
    const wrapper = mount(UpdateBadge, { props: { update: update({ status: 'pinned', risk_score: 90 }) } })
    expect(wrapper.attributes('style')).toContain(`color: ${muted}`)
    expect(wrapper.text()).toBe('Pinned')
  })

  it('keeps the labels tied to the type', () => {
    expect(mount(UpdateBadge, { props: { update: update({ update_type: 'major' }) } }).text()).toBe('18.7 → 18.8')
    expect(mount(UpdateBadge, { props: { update: update({ update_type: 'digest_only' }) } }).text()).toBe('New digest')
    expect(mount(UpdateBadge, { props: { update: update({ update_type: 'other' }) } }).text()).toBe('Update')
  })
})
