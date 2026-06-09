// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package escalation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverlap_BothEmpty_AllFilters_SharedChannel(t *testing.T) {
	a := &Policy{
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1", "2"}}},
	}
	b := &Policy{
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"2", "3"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Len(t, warnings, 1)
	assert.Equal(t, []string{"2"}, warnings[0].SharedChannels)
}

func TestOverlap_NoSharedChannel(t *testing.T) {
	a := &Policy{
		Filters: Filters{Severities: []string{"critical"}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	b := &Policy{
		Filters: Filters{Severities: []string{"critical"}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"2"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_DisjointSeverities(t *testing.T) {
	a := &Policy{
		Filters: Filters{Severities: []string{"warning"}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	b := &Policy{
		Filters: Filters{Severities: []string{"critical"}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_OneEmptyFilters_IntersectsAll(t *testing.T) {
	a := &Policy{
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	b := &Policy{
		Filters: Filters{Severities: []string{"critical"}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Len(t, warnings, 1)
}

func TestOverlap_SkipsSelf(t *testing.T) {
	a := &Policy{
		ID:      "1",
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	b := &Policy{
		ID:      "1",
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"1"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Empty(t, warnings)
}

func TestOverlap_MultiLevelSharedChannel(t *testing.T) {
	a := &Policy{
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels: []Level{
			{ChannelIDs: []string{"10"}},
			{ChannelIDs: []string{"5"}},
		},
	}
	b := &Policy{
		Filters: Filters{Severities: []string{}, Scopes: []Scope{}, Tags: []string{}},
		Levels:  []Level{{ChannelIDs: []string{"5", "6"}}},
	}
	warnings := DetectOverlap(a, []*Policy{b})
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].SharedChannels, "5")
}
