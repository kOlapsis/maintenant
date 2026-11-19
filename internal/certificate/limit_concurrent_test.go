// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package certificate_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/certificate"
	"github.com/kolapsis/maintenant/internal/extension"
)

func TestCreateStandalone_ConcurrentCallsFillTheCapOnce(t *testing.T) {
	pinEdition(t, extension.Community)
	b := newLimitBench(t)
	b.declare("ext-a", hosts("cert", 1, 4)...)

	var created, refused atomic.Int32
	var wg sync.WaitGroup
	for port := 1; port <= 12; port++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := b.svc.CreateStandalone(b.ctx, certificate.CreateCertificateInput{Hostname: "127.0.0.1", Port: port, CheckIntervalSeconds: 3600})
			switch err {
			case nil:
				created.Add(1)
			case certificate.ErrLimitReached:
				refused.Add(1)
			default:
				t.Errorf("port %d: %v", port, err)
			}
		}()
	}
	wg.Wait()

	assert.EqualValues(t, 1, created.Load(), "one place is left under the cap of five")
	assert.EqualValues(t, 11, refused.Load())
}
