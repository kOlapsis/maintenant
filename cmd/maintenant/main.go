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
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kolapsis/maintenant/internal/app"
	_ "github.com/kolapsis/maintenant/internal/kubernetes"
)

var (
	version      = "dev"
	commit       = "unknown"
	buildDate    = "unknown"
	publicKeyB64 = ""
)

func main() {
	mcpStdio := len(os.Args) > 1 && os.Args[1] == "--mcp-stdio"

	logLevel := logLevelFromEnv()
	logOutput := os.Stdout
	if mcpStdio {
		logOutput = os.Stderr
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{
		Level: logLevel,
	}))
	logger.Info("maintenant starting", "version", version, "commit", commit, "build_date", buildDate)

	cfg := app.ConfigFromEnv()
	cfg.Version = version
	cfg.Commit = commit
	cfg.BuildDate = buildDate
	cfg.PublicKeyB64 = publicKeyB64

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	if mcpStdio {
		if err := application.RunMCPStdio(ctx); err != nil {
			logger.Error("MCP stdio server error", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := application.Start(ctx); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}

func logLevelFromEnv() slog.Level {
	switch os.Getenv("MAINTENANT_LOG_LEVEL") {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
