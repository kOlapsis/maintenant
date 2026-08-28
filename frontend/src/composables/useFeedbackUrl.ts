// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import { computed } from 'vue'
import { useEdition } from '@/composables/useEdition'
import { useAppVersion } from '@/composables/useAppVersion'

export const FEEDBACK_URL = 'https://maintenant.dev/feedback/'

/**
 * The version placeholder useAppVersion shows until /api/v1/health answers.
 * Passing it along would tag the feedback with a meaningless value.
 */
const VERSION_PENDING = '...'

/**
 * The feedback form, tagged with the running edition and version so the
 * answer can be read in context. It is a plain link the browser opens in a
 * new tab: the instance itself never contacts maintenant.dev.
 */
export function buildFeedbackUrl(edition: string, version: string): string {
  const params = new URLSearchParams({ source: 'app', edition })
  if (version && version !== VERSION_PENDING) params.set('version', version)
  return `${FEEDBACK_URL}?${params.toString()}`
}

export function useFeedbackUrl() {
  const { editionName } = useEdition()
  const { version } = useAppVersion()
  const feedbackUrl = computed(() => buildFeedbackUrl(editionName.value, version.value))
  return { feedbackUrl }
}
