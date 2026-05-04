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
	"fmt"
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
	args := os.Args[1:]

	// Intercept --mcp-stdio before flag parsing (existing behaviour preserved).
	mcpStdio := len(args) > 0 && args[0] == "--mcp-stdio"

	// Intercept --help / -h and --version / -v before logger init so that
	// slog JSON output does not pollute the human-readable output.
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			cfg := app.ConfigFromEnv()
			app.PrintHelp(os.Stdout, cfg)
			os.Exit(0)
		case "--version", "-v":
			fmt.Printf("maintenant version %s commit %s build %s\n", version, commit, buildDate)
			os.Exit(0)
		}
	}

	// Parse CLI flags — exit 2 on unknown flag or bad value.
	visited, err := app.ParseFlagsOrDie(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	// Build config: env first, CLI overrides second.
	cfg := app.ConfigFromEnv()
	cfg.Version = version
	cfg.Commit = commit
	cfg.BuildDate = buildDate
	cfg.PublicKeyB64 = publicKeyB64

	if err := app.MergeArgsIntoConfig(&cfg, visited); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	logOutput := os.Stdout
	if mcpStdio {
		logOutput = os.Stderr
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{
		Level: slogLevelFrom(cfg.LogLevel),
	}))
	logger.Info("maintenant starting", "version", version, "commit", commit, "build_date", buildDate)

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

func slogLevelFrom(level string) slog.Level {
	switch level {
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
