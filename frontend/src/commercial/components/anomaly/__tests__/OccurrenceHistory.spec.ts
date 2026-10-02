// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import OccurrenceHistory from '@/commercial/components/anomaly/OccurrenceHistory.vue'
import SeasonalityTuner from '@/commercial/components/anomaly/SeasonalityTuner.vue'
import type { AnomalyEventItem, AnomalySettings } from '@/commercial/services/anomalyApi'

// Anchored on a whole hour so the recurrence summary is not at the mercy of the
// minute the suite happens to run at.
const anchor = new Date(2026, 7, 12, 9, 0, 0).getTime() / 1000

function occurrence(hoursAgo: number, partial: Partial<AnomalyEventItem> = {}): AnomalyEventItem {
  const started = anchor - hoursAgo * 3600
  return {
    id: `ev-${hoursAgo}`,
    scope_type: 'container',
    scope_id: 'compose/immich/database',
    metric: 'memory',
    dimension: '',
    node_id: '',
    detector: 'change_point',
    tier: 'active',
    started_at: started,
    ended_at: started + 3600,
    peak_value: 338_690_000,
    baseline_median: 145_000_000,
    peak_deviation: 7.5,
    ...partial,
  }
}

describe('OccurrenceHistory', () => {
  it('names the recurrence when every occurrence lands on the same hour', () => {
    // Same wall-clock hour on four consecutive days: the signature of a baseline
    // problem rather than a workload one.
    const events = [0, 24, 48, 72].map((h) => occurrence(h))
    const w = mount(OccurrenceHistory, { props: { events } })
    expect(w.text()).toContain('always at 09:00')
    expect(w.text()).toContain('4 times')
  })

  it('reports an hourly cadence when occurrences fill the span', () => {
    const events = [0, 1, 2, 3, 4, 5].map((h) => occurrence(h))
    const w = mount(OccurrenceHistory, { props: { events } })
    expect(w.text()).toContain('roughly every hour')
  })

  it('stays silent on a pattern with too few occurrences to claim one', () => {
    const w = mount(OccurrenceHistory, { props: { events: [occurrence(0), occurrence(5)] } })
    expect(w.text()).not.toContain('times in')
  })

  it('spells the peak out against what the series expected', () => {
    const w = mount(OccurrenceHistory, { props: { events: [occurrence(0)] } })
    expect(w.text()).toContain('323.0 MB')
    expect(w.text()).toContain('138.3 MB')
    expect(w.text()).toContain('7.5× the learned spread')
  })

  it('renders nothing without occurrences', () => {
    const w = mount(OccurrenceHistory, { props: { events: [] } })
    expect(w.text()).toBe('')
  })
})

function settings(bucketPull: number): AnomalySettings {
  return { bucket_pull: bucketPull, min_bucket_pull: 0, max_bucket_pull: 20, min_samples: 4, alert_severity: 'warning', updated_at: 0 }
}

describe('SeasonalityTuner', () => {
  it('explains how much of its own level a fully observed hour keeps', () => {
    // pull 3 against 4 samples: 4/7 of the hour's own level.
    const w = mount(SeasonalityTuner, { props: { settings: settings(3) } })
    expect(w.text()).toContain('57%')
  })

  it('says plainly when the hour decides alone', () => {
    const w = mount(SeasonalityTuner, { props: { settings: settings(0) } })
    expect(w.text()).toContain('decides entirely on its own')
  })

  it('emits the pull, not the slider position', () => {
    const w = mount(SeasonalityTuner, { props: { settings: settings(3) } })
    const input = w.find('input[type="range"]')
    // The slider runs backwards: far right means trust the hour, i.e. pull 0.
    input.setValue(20)
    input.trigger('change')
    expect(w.emitted('change')?.[0]).toEqual([0])
  })
})
