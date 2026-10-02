// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AnomalyBadge from '@/commercial/components/anomaly/AnomalyBadge.vue'
import AnomalyFeed from '@/commercial/components/anomaly/AnomalyFeed.vue'
import LearningScreen from '@/commercial/components/anomaly/LearningScreen.vue'
import SeriesDetail from '@/commercial/components/anomaly/SeriesDetail.vue'
import type { AnomalyEventItem, SeriesItem } from '@/commercial/services/anomalyApi'

function series(partial: Partial<SeriesItem>): SeriesItem {
  return {
    scope_type: 'container',
    scope_id: 'compose/app/web',
    metric: 'cpu',
    dimension: '',
    node_id: '',
    state: 'learning',
    progress: 0,
    sensitivity: 'medium',
    metrics: ['cpu', 'memory'],
    current_score: 0,
    ready_at: null,
    estimated_ready_at: null,
    ...partial,
  }
}

describe('AnomalyBadge', () => {
  it('shows learning progress', () => {
    const w = mount(AnomalyBadge, { props: { state: 'learning', progress: 0.42 } })
    expect(w.text()).toContain('Learning 42%')
  })

  it('shows an anomaly label for a ready series with an active anomaly', () => {
    const w = mount(AnomalyBadge, { props: { state: 'ready', active: true } })
    expect(w.text()).toContain('Anomaly')
  })

  it('shows nominal for a ready series with no anomaly', () => {
    const w = mount(AnomalyBadge, { props: { state: 'ready', active: false } })
    expect(w.text()).toContain('Nominal')
  })
})

function event(partial: Partial<AnomalyEventItem> = {}): AnomalyEventItem {
  return {
    id: 'ev-1',
    scope_type: 'container',
    scope_id: 'compose/immich/database',
    metric: 'memory',
    dimension: '',
    node_id: '',
    detector: 'change_point',
    tier: 'active',
    started_at: Math.floor(Date.now() / 1000) - 40,
    ended_at: null,
    peak_value: 120_890_709.3,
    baseline_median: 60_000_000,
    peak_deviation: 7.5,
    ...partial,
  }
}

describe('AnomalyFeed', () => {
  it('renders the peak in the metric own unit rather than raw bytes', () => {
    const w = mount(AnomalyFeed, { props: { events: [event()] } })
    expect(w.text()).toContain('115.3 MB')
    expect(w.text()).not.toContain('120890709')
  })

  it('renders a cpu peak as a percentage', () => {
    const w = mount(AnomalyFeed, { props: { events: [event({ metric: 'cpu', peak_value: 98.42 })] } })
    expect(w.text()).toContain('98.4%')
  })

  it('says what the tier means instead of printing it raw', () => {
    const active = mount(AnomalyFeed, { props: { events: [event()] } })
    expect(active.text()).toContain('alert raised')

    const passive = mount(AnomalyFeed, { props: { events: [event({ tier: 'passive' })] } })
    expect(passive.text()).toContain('flagged only')
  })
})

describe('SeriesDetail', () => {
  const metrics = [
    series({ metric: 'cpu', state: 'ready', progress: 1 }),
    series({ metric: 'memory', state: 'ready', progress: 1 }),
  ]

  it('reports an anomaly on the scope when one of its metrics is open', () => {
    const w = mount(SeriesDetail, {
      props: { scopeId: 'compose/immich/database', metrics, anomalousMetrics: ['memory'] },
    })
    expect(w.text()).toContain('Anomaly')
  })

  it('stays nominal when no anomaly is open', () => {
    const w = mount(SeriesDetail, {
      props: { scopeId: 'compose/immich/database', metrics, anomalousMetrics: [] },
    })
    expect(w.text()).toContain('Nominal')
    expect(w.text()).not.toContain('Anomaly')
  })

  it('shows each metric own state instead of a score nothing computes', () => {
    const w = mount(SeriesDetail, {
      props: {
        scopeId: 'compose/immich/database',
        metrics: [
          series({ metric: 'cpu', state: 'ready', progress: 1 }),
          series({ metric: 'memory', state: 'learning', progress: 0.4 }),
        ],
      },
    })
    expect(w.text()).toContain('Learning 40%')
    expect(w.text()).not.toContain('score')
  })
})

describe('LearningScreen', () => {
  it('reports how many scopes are ready', () => {
    const w = mount(LearningScreen, {
      props: {
        series: [
          series({ scope_id: 'compose/app/web', state: 'ready', progress: 1 }),
          series({ scope_id: 'compose/app/db', state: 'learning', progress: 0.5 }),
        ],
      },
    })
    expect(w.text()).toContain('1 of 2 scopes ready')
  })
})
