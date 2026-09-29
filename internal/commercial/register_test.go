// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package commercial

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}

func TestNewEditionSource_NoKeyMeansNoSource(t *testing.T) {
	src, err := extension.NewEditionSource(extension.SourceConfig{})
	require.NoError(t, err)
	assert.Nil(t, src)
}

func TestNewEditionSource_UnusablePublicKeyReturnsAnError(t *testing.T) {
	src, err := extension.NewEditionSource(extension.SourceConfig{LicenseKey: "key", PublicKeyB64: ""})
	require.Error(t, err)
	assert.True(t, src == nil, "a failed source must be an untyped nil")
}
