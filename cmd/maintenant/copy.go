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
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kolapsis/maintenant/internal/store"
)

// Exit codes, per the command's contract.
const (
	copyExitOK       = 0
	copyExitRefused  = 1 // target not empty, source unreadable or behind, or the operator said no
	copyExitFailed   = 2 // failed mid-copy: the transaction rolled back, the target is empty
	copyExitAgentBad = 1 // agent mode never carries a server data set
)

// runCopy carries an existing local install into an empty PostgreSQL database
// and exits. It writes nothing to the source, announces what it will and will
// not carry before touching the target, and rolls everything back on failure.
func runCopy(dbPath, targetDSN string, assumeYes bool, out io.Writer, in io.Reader, logger *slog.Logger) int {
	ctx := context.Background()

	if _, err := store.ParseDSN(targetDSN); err != nil {
		logStorageStartupError(logger, err, targetDSN)
		return copyExitRefused
	}

	// Read-only on the source: the original install stays usable whatever
	// happens here (FR-028).
	src, err := sql.Open("sqlite3_maintenant", "file:"+dbPath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		logger.Error("cannot open the source database", "path", dbPath, "error", err)
		return copyExitRefused
	}
	defer func() { _ = src.Close() }()
	if err := src.PingContext(ctx); err != nil {
		logger.Error("cannot read the source database",
			"path", dbPath, "error", err,
			"fix", "check the path, and that this is the file the instance uses")
		return copyExitRefused
	}

	dst, err := sql.Open("pgx", store.ApplyDefaultSSLMode(targetDSN))
	if err != nil {
		logStorageStartupError(logger, fmt.Errorf("%w", store.ErrInvalidDSN), targetDSN)
		return copyExitRefused
	}
	defer func() { _ = dst.Close() }()
	if err := dst.PingContext(ctx); err != nil {
		logger.Error("the target database does not answer",
			"target", store.RedactDSN(targetDSN),
			"fix", "check the host, port, credentials and network route")
		return copyExitRefused
	}

	_, _ = fmt.Fprintf(out, "\nCopying %s\n     to %s\n\n", dbPath, store.RedactDSN(targetDSN))

	confirm := func(store.Plan) bool {
		if assumeYes {
			return true
		}
		_, _ = fmt.Fprint(out, "Proceed? [y/N] ")
		reader := bufio.NewReader(in)
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}

	report, err := store.Copy(ctx, src, dst, out, confirm)
	switch {
	case errors.Is(err, store.ErrCopyRefused):
		_, _ = fmt.Fprintln(out, "Nothing was written.")
		return copyExitRefused
	case errors.Is(err, store.ErrTargetNotEmpty):
		logger.Error("the target database is not empty", "error", err,
			"fix", "point at an empty database: merging would need conflict rules nothing can settle")
		return copyExitRefused
	case errors.Is(err, store.ErrSourceBehind):
		logger.Error("the source database is behind this binary's schema", "error", err,
			"fix", "start this binary once on the source so it migrates, then copy")
		return copyExitRefused
	case err != nil:
		logger.Error("the copy failed and was rolled back", "error", err,
			"state", "the target is empty again and the source was not written to; you can retry")
		return copyExitFailed
	}

	_, _ = fmt.Fprintf(out, "Copied %d rows across %d tables, counts verified:\n\n",
		report.Plan.Total(), len(report.Copied))
	for _, c := range report.Copied {
		_, _ = fmt.Fprintf(out, "  %-28s %8d\n", c.Table, c.Rows)
	}
	_, _ = fmt.Fprintf(out, "\nNext: set MAINTENANT_DATABASE_URL to this database and restart.\n"+
		"The agents reconnect on their own; there is nothing to do on the monitored machines.\n\n")
	return copyExitOK
}
