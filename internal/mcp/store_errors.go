// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"log/slog"

	"github.com/kolapsis/maintenant/internal/store"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// storeErrResult turns a failed store call into a tool error that never carries the driver's text.
func storeErrResult(err error, action string) (*gomcp.CallToolResult, any, error) {
	switch {
	case store.IsUnavailable(err):
		return errResult(action + " failed: storage temporarily unavailable")
	case store.IsUniqueViolation(err):
		return errResult(action + " failed: a record with the same name already exists")
	}
	slog.Error("mcp store error", "action", action, "error", err)
	return errResult(action + " failed: internal error")
}
