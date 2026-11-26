// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMigrateLeavesNoPooledConnectionHeld(t *testing.T) {
	db := openTestDB(t)

	assert.Zero(t, db.db.Stats().InUse)
}
