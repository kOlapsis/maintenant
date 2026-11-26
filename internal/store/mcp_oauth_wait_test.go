// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/mcp/oauth"
)

func TestMCPOAuth_ARequestGivesUpWaitingForTheSecretCheckWhenItsContextEnds(t *testing.T) {
	s := NewMCPOAuthStore(openTestDB(t))
	s.BindClientSecret(oauth.HashToken("secret-A"))
	s.gate <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.GetToken(ctx, "any")
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("the request still waits behind the check of another request")
	}
}
