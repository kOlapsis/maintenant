// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { describe, it, expect, beforeEach, vi } from 'vitest'
import { ref } from 'vue'

// Mock the two composables the URL reads from, so the test drives edition and
// version directly without an edition fetch or a /api/v1/health call.
const state = vi.hoisted(() => ({
  edition: { value: 'community' },
  version: { value: '...' },
}))

vi.mock('@/composables/useEdition', () => ({ useEdition: () => ({ editionName: state.edition }) }))
vi.mock('@/composables/useAppVersion', () => ({ useAppVersion: () => ({ version: state.version }) }))

import { buildFeedbackUrl, useFeedbackUrl, FEEDBACK_URL } from '../useFeedbackUrl'

beforeEach(() => {
  state.edition = ref('community')
  state.version = ref('...')
})

describe('buildFeedbackUrl', () => {
  it('tags the form with source, edition and version', () => {
    const url = new URL(buildFeedbackUrl('pro', '1.4.0'))
    expect(url.origin + url.pathname).toBe(FEEDBACK_URL)
    expect(url.searchParams.get('source')).toBe('app')
    expect(url.searchParams.get('edition')).toBe('pro')
    expect(url.searchParams.get('version')).toBe('1.4.0')
  })

  it('leaves version out while it is still the loading placeholder', () => {
    const url = new URL(buildFeedbackUrl('community', '...'))
    expect(url.searchParams.get('edition')).toBe('community')
    expect(url.searchParams.has('version')).toBe(false)
  })

  it('leaves version out when it is empty', () => {
    expect(new URL(buildFeedbackUrl('personal', '')).searchParams.has('version')).toBe(false)
  })

  it('encodes the parameters', () => {
    const url = buildFeedbackUrl('a b&c', '1.0.0+dev')
    expect(url).toBe(`${FEEDBACK_URL}?source=app&edition=a+b%26c&version=1.0.0%2Bdev`)
  })
})

describe('useFeedbackUrl', () => {
  it('follows the running edition and adds the version once it is known', () => {
    state.edition = ref('personal')
    const { feedbackUrl } = useFeedbackUrl()

    expect(feedbackUrl.value).toBe(`${FEEDBACK_URL}?source=app&edition=personal`)

    state.version.value = '1.4.0'
    expect(feedbackUrl.value).toBe(`${FEEDBACK_URL}?source=app&edition=personal&version=1.4.0`)

    state.edition.value = 'pro'
    expect(feedbackUrl.value).toBe(`${FEEDBACK_URL}?source=app&edition=pro&version=1.4.0`)
  })
})
