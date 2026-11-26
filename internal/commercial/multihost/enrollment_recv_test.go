// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package multihost

import (
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

type endlessStream struct {
	grpc.ServerStream
}

func (endlessStream) Recv() (*agentpb.ClientMessage, error) { return &agentpb.ClientMessage{}, nil }

func (endlessStream) Send(*agentpb.ServerMessage) error { return nil }

func TestForwardRecv_StopsWhenTheHandlerIsGone(t *testing.T) {
	unread := make(chan recvResult, 1)
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		forwardRecv(endlessStream{}, unread, done)
		close(exited)
	}()

	close(done)

	select {
	case <-exited:
	case <-time.After(2 * time.Second):
		t.Fatal("the receive goroutine is still blocked on a channel nobody reads")
	}
}
