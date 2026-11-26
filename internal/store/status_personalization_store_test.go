// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func footerLinkOrder(t *testing.T, s *PersonalizationStoreImpl) map[string]int {
	t.Helper()
	links, err := s.ListFooterLinks(context.Background())
	require.NoError(t, err)
	got := make(map[string]int, len(links))
	for _, l := range links {
		got[l.ID] = l.Position
	}
	return got
}

func faqItemOrder(t *testing.T, s *PersonalizationStoreImpl) map[string]int {
	t.Helper()
	items, err := s.ListFAQItems(context.Background())
	require.NoError(t, err)
	got := make(map[string]int, len(items))
	for _, it := range items {
		got[it.ID] = it.Position
	}
	return got
}

func TestPersonalizationStore_ReorderFooterLinksAppliesNothingOnFailure(t *testing.T) {
	ctx := context.Background()
	s := NewPersonalizationStore(openTestDB(t))
	var ids []string
	for _, label := range []string{"a", "b", "c", "gone"} {
		l, err := s.CreateFooterLink(ctx, label, "https://example.com/"+label)
		require.NoError(t, err)
		ids = append(ids, l.ID)
	}
	a, b, c, gone := ids[0], ids[1], ids[2], ids[3]
	require.NoError(t, s.DeleteFooterLink(ctx, gone))
	before := map[string]int{a: 0, b: 1, c: 2}

	tests := map[string][]string{
		"unknown id":  {c, b, gone, a},
		"repeated id": {c, c, b, a},
	}
	for name, order := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := s.ReorderFooterLinks(ctx, order)
			require.Error(t, err)
			assert.Equal(t, before, footerLinkOrder(t, s))
		})
	}

	links, err := s.ReorderFooterLinks(ctx, []string{c, a, b})
	require.NoError(t, err)
	require.Len(t, links, 3)
	assert.Equal(t, map[string]int{c: 0, a: 1, b: 2}, footerLinkOrder(t, s))
}

func TestPersonalizationStore_ReorderFAQItemsAppliesNothingOnFailure(t *testing.T) {
	ctx := context.Background()
	s := NewPersonalizationStore(openTestDB(t))
	var ids []string
	for _, q := range []string{"a", "b", "c", "gone"} {
		it, err := s.CreateFAQItem(ctx, q, "answer", "<p>answer</p>")
		require.NoError(t, err)
		ids = append(ids, it.ID)
	}
	a, b, c, gone := ids[0], ids[1], ids[2], ids[3]
	require.NoError(t, s.DeleteFAQItem(ctx, gone))
	before := map[string]int{a: 0, b: 1, c: 2}

	tests := map[string][]string{
		"unknown id":  {c, b, gone, a},
		"repeated id": {c, c, b, a},
	}
	for name, order := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := s.ReorderFAQItems(ctx, order)
			require.Error(t, err)
			assert.Equal(t, before, faqItemOrder(t, s))
		})
	}

	items, err := s.ReorderFAQItems(ctx, []string{c, a, b})
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, map[string]int{c: 0, a: 1, b: 2}, faqItemOrder(t, s))
}
