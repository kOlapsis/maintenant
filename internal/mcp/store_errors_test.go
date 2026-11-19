// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/alert"
	"github.com/kolapsis/maintenant/internal/alert/escalation"
	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/store"
	"github.com/kolapsis/maintenant/internal/store/storetest"
)

var (
	errSQLText = errors.New(`insert into escalation_policies: SQLSTATE 42P01 relation "escalation_policies" does not exist`)
	errOutage  = &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	errUnique  = &pgconn.PgError{Code: "23505", Message: `duplicate key value violates unique constraint "idx_triggers_name"`}
)

func assertNoStoreText(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, leak := range []string{"sqlstate", "escalation_policies", "duplicate key", "idx_triggers_name", "connection refused", "unique constraint"} {
		assert.NotContains(t, lower, leak)
	}
}

func TestTriggerTools_ANameConflictNeverShowsSQL(t *testing.T) {
	withEdition(t, extension.Pro)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db := storetest.Open(t, logger)
	channels := store.NewChannelStore(db)
	svc := &Services{Channels: channels, Triggers: store.NewTriggerStore(db), Logger: logger, Version: "test"}
	ctx := context.Background()
	ch := &alert.NotificationChannel{Name: "ops", Type: "webhook", URL: "https://example.com/hook", Enabled: true}
	_, err := channels.InsertChannel(ctx, ch)
	require.NoError(t, err)

	first, _, err := createTriggerHandler(svc)(ctx, nil, triggerInput{Name: "dup", ChannelIDs: []string{ch.ID}})
	require.NoError(t, err)
	require.False(t, first.IsError, textFromContent(t, first.Content))

	second, _, err := createTriggerHandler(svc)(ctx, nil, triggerInput{Name: "dup", ChannelIDs: []string{ch.ID}})

	require.NoError(t, err, "a store failure is a tool result, not an error the SDK would copy to the client")
	assert.True(t, second.IsError)
	assert.Contains(t, textFromContent(t, second.Content), "already exists")
	assertNoStoreText(t, textFromContent(t, second.Content))
}

type failingTriggers struct {
	alert.TriggerStore
	err error
}

func (f failingTriggers) ListTriggers(context.Context) ([]*alert.AlertTrigger, error) {
	return nil, f.err
}

func TestTriggerTools_AStoreFailureNeverShowsSQL(t *testing.T) {
	for name, tc := range map[string]struct {
		err      error
		contains string
	}{
		"sql":    {errSQLText, "internal error"},
		"outage": {errOutage, "unavailable"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &Services{Triggers: failingTriggers{err: tc.err}}

			result, _, err := listTriggersHandler(svc)(context.Background(), nil, listTriggersInput{})

			require.NoError(t, err)
			assert.True(t, result.IsError)
			assert.Contains(t, textFromContent(t, result.Content), tc.contains)
			assertNoStoreText(t, textFromContent(t, result.Content))
		})
	}
}

type failingEscalation struct {
	escalation.Service
	err     error
	listErr error
}

func (f failingEscalation) CreatePolicy(context.Context, escalation.PolicyRequest) (*escalation.Policy, error) {
	return nil, f.err
}
func (f failingEscalation) ListPolicies(context.Context, bool) ([]*escalation.Policy, error) {
	return nil, f.listErr
}
func (f failingEscalation) GetPolicy(context.Context, string) (*escalation.Policy, error) {
	return nil, f.err
}
func (f failingEscalation) DeletePolicy(context.Context, string) error { return f.err }
func (f failingEscalation) GetRun(context.Context, string) (*escalation.Run, error) {
	return nil, f.err
}

func TestEscalationTools_StoreFailuresNeverShowSQL(t *testing.T) {
	withEdition(t, extension.Pro)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		err      error
		contains string
	}{
		"sql":      {errSQLText, "internal error"},
		"unique":   {errUnique, "already exists"},
		"outage":   {errOutage, "unavailable"},
		"notfound": {escalation.ErrPolicyNotFound, "policy_not_found"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &Services{EscalationSvc: failingEscalation{err: tc.err}}
			for tool, call := range map[string]func() (string, bool){
				"create": func() (string, bool) {
					r, _, err := createEscalationPolicyHandler(svc)(ctx, nil, createEscalationPolicyInput{Name: "p"})
					require.NoError(t, err)
					return textFromContent(t, r.Content), r.IsError
				},
				"get": func() (string, bool) {
					r, _, err := getEscalationPolicyHandler(svc)(ctx, nil, getEscalationPolicyInput{ID: "x"})
					require.NoError(t, err)
					return textFromContent(t, r.Content), r.IsError
				},
				"delete": func() (string, bool) {
					r, _, err := deleteEscalationPolicyHandler(svc)(ctx, nil, deleteEscalationPolicyInput{ID: "x"})
					require.NoError(t, err)
					return textFromContent(t, r.Content), r.IsError
				},
			} {
				text, isErr := call()
				assert.True(t, isErr, tool)
				assert.Contains(t, text, tc.contains, tool)
				assertNoStoreText(t, text)
			}
		})
	}
}

func TestEscalationTools_UnknownRunIsStillRunNotFound(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{EscalationSvc: failingEscalation{err: escalation.ErrRunNotFound}}

	r, _, err := getEscalationRunHandler(svc)(context.Background(), nil, getEscalationRunInput{ID: "x"})

	require.NoError(t, err)
	assert.Contains(t, textFromContent(t, r.Content), "run_not_found")
}

func TestEscalationTools_ACreationThatCannotListPoliciesNeverShowsSQL(t *testing.T) {
	withEdition(t, extension.Pro)
	svc := &Services{EscalationSvc: failingEscalation{listErr: errSQLText}}

	r, _, err := createEscalationPolicyHandler(svc)(context.Background(), nil, createEscalationPolicyInput{Name: "p"})

	require.NoError(t, err)
	assert.True(t, r.IsError)
	assertNoStoreText(t, textFromContent(t, r.Content))
}
