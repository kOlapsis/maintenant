// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientKeepaliveMatchesContract(t *testing.T) {
	require.Equal(t, 20*time.Second, clientKeepalive.Time)
	require.Equal(t, 10*time.Second, clientKeepalive.Timeout)
	require.True(t, clientKeepalive.PermitWithoutStream)
}
