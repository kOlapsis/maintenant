// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance test of 048-heartbeat-edit, FR-011. A floor: the wording itself is reviewed by hand.
func TestHeartbeatDocsExplainTheEditAndTheDeadlineRecomputation(t *testing.T) {
	doc := repoFile(t, "docs", "features", "heartbeats.md")

	heading := regexp.MustCompile(`(?mi)^(#{2,4})\s+[^\n]*\bedit`).FindStringSubmatchIndex(doc)
	require.NotNil(t, heading, "a section of the heartbeat documentation is about editing a heartbeat")
	section := doc[heading[0]:]
	level := doc[heading[2]:heading[3]]
	if nl := strings.IndexByte(section, '\n'); nl >= 0 {
		if next := regexp.MustCompile(`(?m)^#{2,` + strconv.Itoa(len(level)) + `}\s`).FindStringIndex(section[nl:]); next != nil {
			section = section[:nl+next[0]]
		}
	}
	lower := strings.ToLower(section)

	assert.Contains(t, lower, "ping url", "the section says what happens to the ping URL")
	assert.Contains(t, lower, "deadline", "the section explains the deadline")
	assert.Contains(t, lower, "last ping", "the deadline counts from the last ping")
	assert.Contains(t, lower, "resum", "the deadline counts from the resume after a pause")
	assert.Contains(t, lower, "start", "the deadline counts from the start of the current run")
}
