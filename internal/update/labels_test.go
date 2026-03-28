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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- US1: tag-include label parsing ---

func TestParseUpdateLabels_TagInclude_ValidRegex(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-include": `^20\.\d+\.\d+-alpine$`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	require.NotNil(t, cfg.TagInclude)
	assert.True(t, cfg.TagInclude.MatchString("20.12.0-alpine"))
	assert.False(t, cfg.TagInclude.MatchString("21.0.0-alpine"))
}

func TestParseUpdateLabels_TagInclude_InvalidRegex_ReturnsNil(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-include": `[unclosed`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	assert.Nil(t, cfg.TagInclude)
}

func TestParseUpdateLabels_TagInclude_EmptyString_ReturnsNil(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-include": "",
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	assert.Nil(t, cfg.TagInclude)
}

func TestParseUpdateLabels_TagInclude_Absent_ReturnsNil(t *testing.T) {
	cfg := ParseUpdateLabels(map[string]string{}, testLogger())
	assert.Nil(t, cfg.TagInclude)
}

// --- US1: tag-exclude label parsing ---

func TestParseUpdateLabels_TagExclude_ValidRegex(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-exclude": `(rc|beta|alpha)`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	require.NotNil(t, cfg.TagExclude)
	assert.True(t, cfg.TagExclude.MatchString("2.0-rc1"))
	assert.False(t, cfg.TagExclude.MatchString("2.0"))
}

func TestParseUpdateLabels_TagExclude_InvalidRegex_ReturnsNil(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-exclude": `[bad`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	assert.Nil(t, cfg.TagExclude)
}

func TestParseUpdateLabels_TagExclude_EmptyString_ReturnsNil(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-exclude": "",
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	assert.Nil(t, cfg.TagExclude)
}

func TestParseUpdateLabels_TagExclude_Absent_ReturnsNil(t *testing.T) {
	cfg := ParseUpdateLabels(map[string]string{}, testLogger())
	assert.Nil(t, cfg.TagExclude)
}

// --- US4: invalid regex handling — partial validity ---

func TestParseUpdateLabels_ValidInclude_InvalidExclude(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-include": `^20\.`,
		"maintenant.update.tag-exclude": `[bad`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	require.NotNil(t, cfg.TagInclude, "include should be compiled")
	assert.Nil(t, cfg.TagExclude, "invalid exclude should be nil")
}

func TestParseUpdateLabels_InvalidInclude_ValidExclude(t *testing.T) {
	labels := map[string]string{
		"maintenant.update.tag-include": `[bad`,
		"maintenant.update.tag-exclude": `(rc|beta)`,
	}
	cfg := ParseUpdateLabels(labels, testLogger())
	assert.Nil(t, cfg.TagInclude, "invalid include should be nil")
	require.NotNil(t, cfg.TagExclude, "exclude should be compiled")
}
