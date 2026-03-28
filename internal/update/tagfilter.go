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
	"strings"
)

// TagFilter filters registry tag lists based on include/exclude regex patterns
// and the automatic variant suffix (e.g. "-alpine").
//
// Priority rules:
//  1. If include is set, only matching tags are kept (variant filter is skipped).
//  2. If include is nil and variant is non-empty, the automatic variant filter applies.
//  3. If exclude is set, matching tags are removed from the remaining set.
//  4. Exclude always wins — applied last.
type TagFilter struct {
	include *regexp.Regexp
	exclude *regexp.Regexp
	variant string
}

// NewTagFilter creates a TagFilter from optional include/exclude patterns and a variant suffix.
// Pass nil for include or exclude to disable those filters.
// Pass an empty string for variant to disable the automatic variant filter.
func NewTagFilter(include, exclude *regexp.Regexp, variant string) *TagFilter {
	return &TagFilter{
		include: include,
		exclude: exclude,
		variant: variant,
	}
}

// Filter returns the subset of tags that pass the configured filters.
//
// When include is set:
//   - keeps only tags matching the include regex
//   - the automatic variant filter is NOT applied (include takes full control)
//
// When include is nil:
//   - applies the automatic variant filter if variant is non-empty
//   - all tags pass when variant is empty
//
// After include/variant, exclude removes any remaining matching tags.
func (f *TagFilter) Filter(tags []string) []string {
	var result []string

	if f.include != nil {
		// include is set: it replaces the variant filter entirely
		for _, tag := range tags {
			if f.include.MatchString(tag) {
				result = append(result, tag)
			}
		}
	} else if f.variant != "" {
		// no include: apply automatic variant filter
		for _, tag := range tags {
			_, tv := splitVariant(tag)
			if strings.EqualFold(tv, f.variant) {
				result = append(result, tag)
			}
		}
	} else {
		// no include, no variant: all tags pass
		result = make([]string, len(tags))
		copy(result, tags)
	}

	if f.exclude == nil {
		return result
	}

	// apply exclude last — exclude always wins
	filtered := result[:0]
	for _, tag := range result {
		if !f.exclude.MatchString(tag) {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}
