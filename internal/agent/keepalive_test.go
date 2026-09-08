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
