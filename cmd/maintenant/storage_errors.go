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

package main

import (
	"errors"
	"log/slog"

	"github.com/kolapsis/maintenant/internal/app"
	"github.com/kolapsis/maintenant/internal/store"
)

// logStorageStartupError names the cause and what to correct, in one of the
// distinct families an operator can act on (FR-018). It never renders the
// connection string: the message says what to fix, the redacted target says
// where (FR-021). Returns false when err is not a storage startup failure.
func logStorageStartupError(logger *slog.Logger, err error, dsn string) bool {
	target := store.RedactDSN(dsn)
	switch {
	case errors.Is(err, app.ErrDatabaseURLInAgentMode):
		logger.Error("MAINTENANT_DATABASE_URL is not accepted in agent mode",
			"fix", "the agent always stores its state locally: drop the setting, or run this instance with --mode=server")
	case errors.Is(err, store.ErrInvalidDSN):
		logger.Error("the database connection string cannot be read",
			"fix", "expected postgres://user:password@host:5432/database[?sslmode=require]")
	case errors.Is(err, store.ErrUnreachable):
		logger.Error("the database does not answer", "target", target,
			"fix", "check the host and port, the network route and the firewall; the instance does not fall back to the local file")
	case errors.Is(err, store.ErrAuthRefused):
		logger.Error("the database refused the credentials", "target", target,
			"fix", "check the user and password, and that this role may connect to this database")
	case errors.Is(err, store.ErrUnsupportedVersion):
		logger.Error("the database version is not supported", "target", target,
			"fix", "PostgreSQL 14 or newer is required")
	case errors.Is(err, store.ErrSchemaNewer):
		logger.Error("the database schema was written by a newer release", "error", err,
			"fix", "run the version that wrote it, or upgrade this binary; it will not write into a schema it does not know")
	default:
		return false
	}
	return true
}
