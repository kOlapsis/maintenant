// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package escalation

import (
	"testing"

	esc "github.com/kolapsis/maintenant/internal/alert/escalation"

	"github.com/stretchr/testify/assert"
)

func TestOverlap_BothEmpty_AllFilters_SharedChannel(t *testing.T) {
	a := &esc.Policy{
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1", "2"}}},
	}
	b := &esc.Policy{
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"2", "3"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Len(t, warnings, 1)
	assert.Equal(t, []string{"2"}, warnings[0].SharedChannels)
}

func TestOverlap_NoSharedChannel(t *testing.T) {
	a := &esc.Policy{
		Filters: esc.Filters{Severities: []string{"critical"}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	b := &esc.Policy{
		Filters: esc.Filters{Severities: []string{"critical"}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"2"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_DisjointSeverities(t *testing.T) {
	a := &esc.Policy{
		Filters: esc.Filters{Severities: []string{"warning"}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	b := &esc.Policy{
		Filters: esc.Filters{Severities: []string{"critical"}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_OneEmptyFilters_IntersectsAll(t *testing.T) {
	a := &esc.Policy{
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	b := &esc.Policy{
		Filters: esc.Filters{Severities: []string{"critical"}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Len(t, warnings, 1)
}

func TestOverlap_SkipsSelf(t *testing.T) {
	a := &esc.Policy{
		ID:      "1",
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	b := &esc.Policy{
		ID:      "1",
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_MultiLevelSharedChannel(t *testing.T) {
	a := &esc.Policy{
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels: []esc.Level{
			{ChannelIDs: []string{"10"}},
			{ChannelIDs: []string{"5"}},
		},
	}
	b := &esc.Policy{
		Filters: esc.Filters{Severities: []string{}, Scopes: []esc.Scope{}, Tags: []string{}},
		Levels:  []esc.Level{{ChannelIDs: []string{"5", "6"}}},
	}
	warnings := DetectOverlap(a, []*esc.Policy{b})
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].SharedChannels, "5")
}
