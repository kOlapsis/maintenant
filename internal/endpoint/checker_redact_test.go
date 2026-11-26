// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckHTTP_ErrorNeverQuotesTheSecretPartsOfTheTarget(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	result := CheckHTTP(context.Background(), &Endpoint{
		EndpointType: TypeHTTP, Config: DefaultConfig(),
		Target: "http://ops:hunter2@" + addr + "/hooks/S3CR3T?token=t0k3n",
	}, slog.Default())

	require.False(t, result.Success)
	require.NotEmpty(t, result.ErrorMessage)
	for _, secret := range []string{"hunter2", "ops:", "S3CR3T", "t0k3n"} {
		assert.NotContains(t, result.ErrorMessage, secret)
	}
	assert.Contains(t, result.ErrorMessage, "http://"+addr)
}
