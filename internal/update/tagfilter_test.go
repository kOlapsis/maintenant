// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package update

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- US1: Include-only ---

func TestTagFilter_IncludeOnly_MatchesPattern(t *testing.T) {
	f := NewTagFilter(regexp.MustCompile(`^20\.\d+\.\d+-alpine$`), nil, "")
	got := f.Filter([]string{"20.11.0-alpine", "20.12.0-alpine", "21.0.0-alpine", "20.12.0", "19.0.0-alpine"})
	assert.Equal(t, []string{"20.11.0-alpine", "20.12.0-alpine"}, got)
}

func TestTagFilter_IncludeOnly_NoMatch_ReturnsEmpty(t *testing.T) {
	f := NewTagFilter(regexp.MustCompile(`^99\.`), nil, "")
	got := f.Filter([]string{"1.0", "2.0", "3.0-alpine"})
	assert.Empty(t, got)
}

func TestTagFilter_IncludeOnly_ReplacesVariantFilter(t *testing.T) {
	// With include set, tags without the variant suffix still pass if include matches them.
	// include=^20\. matches both "20.12.0" and "20.12.0-alpine" regardless of variant.
	f := NewTagFilter(regexp.MustCompile(`^20\.`), nil, "-alpine")
	got := f.Filter([]string{"20.12.0", "20.12.0-alpine", "21.0.0", "21.0.0-alpine"})
	assert.Equal(t, []string{"20.12.0", "20.12.0-alpine"}, got)
}

func TestTagFilter_NilInclude_NilExclude_NoVariant_Passthrough(t *testing.T) {
	f := NewTagFilter(nil, nil, "")
	tags := []string{"1.0", "2.0", "3.0-alpine"}
	got := f.Filter(tags)
	assert.Equal(t, tags, got)
}

// --- T005b: Variant preservation without include ---

func TestTagFilter_NilInclude_WithVariant_AppliesVariantFilter(t *testing.T) {
	// When include is nil and variant is set, only tags matching the variant pass.
	f := NewTagFilter(nil, nil, "-alpine")
	got := f.Filter([]string{"20.11.0-alpine", "20.12.0-alpine", "20.12.0", "21.0.0"})
	assert.Equal(t, []string{"20.11.0-alpine", "20.12.0-alpine"}, got)
}

func TestTagFilter_NilInclude_EmptyVariant_AllTagsPass(t *testing.T) {
	// When include is nil and variant is empty, all tags pass (plain semver).
	f := NewTagFilter(nil, nil, "")
	got := f.Filter([]string{"1.0", "2.0", "2.0-alpine"})
	assert.Equal(t, []string{"1.0", "2.0", "2.0-alpine"}, got)
}

// --- US2: Exclude-only ---

func TestTagFilter_ExcludeOnly_RemovesMatchingTags(t *testing.T) {
	f := NewTagFilter(nil, regexp.MustCompile(`(rc|beta|alpha)`), "")
	got := f.Filter([]string{"1.0", "2.0-rc1", "2.0-beta2", "3.0-alpha", "3.0"})
	assert.Equal(t, []string{"1.0", "3.0"}, got)
}

func TestTagFilter_ExcludeOnly_VariantFilterRemainsActive(t *testing.T) {
	// With exclude-only, the variant filter still applies (not replaced).
	f := NewTagFilter(nil, regexp.MustCompile(`rc`), "-alpine")
	got := f.Filter([]string{"20.11.0-alpine", "20.12.0-rc1-alpine", "20.12.0", "20.12.0-alpine"})
	assert.Equal(t, []string{"20.11.0-alpine", "20.12.0-alpine"}, got)
}

func TestTagFilter_NilExclude_Passthrough(t *testing.T) {
	f := NewTagFilter(nil, nil, "")
	tags := []string{"1.0", "2.0-rc1"}
	assert.Equal(t, tags, f.Filter(tags))
}

// --- US3: Combined include + exclude ---

func TestTagFilter_IncludeAndExclude_IncludeFirst(t *testing.T) {
	// Include selects major branch 20, exclude removes rc/beta.
	f := NewTagFilter(regexp.MustCompile(`^20\.`), regexp.MustCompile(`(rc|beta)`), "")
	got := f.Filter([]string{"20.12.0", "20.13.0-rc1", "20.13.0", "21.0.0"})
	assert.Equal(t, []string{"20.12.0", "20.13.0"}, got)
}

func TestTagFilter_ExcludeMatchesAll_ReturnsEmpty(t *testing.T) {
	// Exclude removes everything that include selected.
	f := NewTagFilter(regexp.MustCompile(`^20\.`), regexp.MustCompile(`^20\.`), "")
	got := f.Filter([]string{"20.12.0", "20.13.0"})
	assert.Empty(t, got)
}

func TestTagFilter_ComplexRegex_GroupsAndAlternation(t *testing.T) {
	f := NewTagFilter(
		regexp.MustCompile(`^(v?[0-9]+\.[0-9]+\.[0-9]+)(-alpine)?$`),
		regexp.MustCompile(`(alpha|beta|rc)`),
		"",
	)
	got := f.Filter([]string{
		"1.0.0", "1.0.0-alpine", "1.1.0-rc1", "2.0.0-beta", "v2.0.0", "v2.0.0-alpine", "latest",
	})
	assert.Equal(t, []string{"1.0.0", "1.0.0-alpine", "v2.0.0", "v2.0.0-alpine"}, got)
}
