// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmbeddedAgentURL_FollowsTheListenerMode(t *testing.T) {
	assert.Equal(t, "grpcs://127.0.0.1:8443", embeddedAgentURL(MultiHostConfig{GRPCListen: "127.0.0.1:8443"}))
	assert.Equal(t, "grpc://127.0.0.1:8443", embeddedAgentURL(MultiHostConfig{GRPCListen: "127.0.0.1:8443", InsecureGRPC: true}),
		"an h2c listener only speaks plaintext")
}
