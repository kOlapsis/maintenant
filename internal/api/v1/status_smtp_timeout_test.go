// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deadlineMailer struct {
	budget      time.Duration
	hasDeadline bool
}

func (m *deadlineMailer) Send(ctx context.Context, _, _, _ string) error {
	deadline, ok := ctx.Deadline()
	m.hasDeadline, m.budget = ok, time.Until(deadline)
	return context.DeadlineExceeded
}

func TestHandleTestSmtp_GivesUpBeforeTheRequestLimit(t *testing.T) {
	mailer := &deadlineMailer{}
	h := NewStatusAdminHandler(nil, nil, nil, nil, nil, nil, mailer)

	rec := httptest.NewRecorder()
	h.HandleTestSmtp(rec, httptest.NewRequest(http.MethodPost, "/api/v1/status/smtp/test", strings.NewReader(`{"to":"ops@example.com"}`)))

	require.True(t, mailer.hasDeadline, "the test send must be bounded")
	assert.Less(t, mailer.budget, 10*time.Second)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Equal(t, "smtp_failed", decodeRefusal(t, rec).Code)
}
