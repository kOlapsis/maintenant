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

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/app"
	_ "github.com/kolapsis/maintenant/internal/kubernetes"
	"github.com/kolapsis/maintenant/internal/trust"
)

var (
	version      = "dev"
	commit       = "unknown"
	buildDate    = "unknown"
	publicKeyB64 = ""
)

// defaultAgentDataDir is where an agent keeps its identity and liveness file
// when MAINTENANT_DATA_DIR is unset.
const defaultAgentDataDir = "/var/lib/maintenant"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	args := os.Args[1:]

	// --mcp-stdio is a mode, not a configuration option: it is read here and
	// dropped from the args so the flag registry never sees an unknown flag.
	mcpStdio := len(args) > 0 && args[0] == "--mcp-stdio"
	if mcpStdio {
		args = args[1:]
	}

	// --help and --version answer before the logger exists, so slog JSON does
	// not land in the middle of human-readable output.
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

	// One parser for every option: app.Registry holds the flag, its environment
	// variable and how it lands in Config, so --help cannot advertise a flag the
	// binary would then refuse. Unknown flag or bad value exits 2.
	visited, err := app.ParseFlagsOrDie(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	// Environment first, CLI second: flags win, as the help text promises.
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
	if cfg.MultiHost.InsecureSkipVerify {
		logger.Warn("insecure TLS verification disabled — do not use in production")
	}
	logger.Info("maintenant starting", "version", version, "commit", commit, "build_date", buildDate, "mode", cfg.Mode)

	// Loaded before anything can probe: both server and agent modes run from
	// here, and a bad path must stop the process rather than turn every TLS
	// check into a misleading "unknown authority".
	if err := trust.Load(cfg.CACertFile); err != nil {
		logger.Error("failed to load extra CA bundle", "error", err)
		os.Exit(1)
	}
	if cfg.CACertFile != "" {
		logger.Info("extra CA bundle trusted", "path", cfg.CACertFile)
	}

	// Checked before the agent branch: an agent handed a connection string
	// must refuse it, not ignore it (FR-003, FR-030).
	if err := cfg.ValidateStorage(); err != nil {
		if !logStorageStartupError(logger, err, cfg.DatabaseURL) {
			logger.Error("invalid storage configuration", "error", err)
		}
		os.Exit(1)
	}

	// --copy-store-to runs the copy and exits, like --mcp-stdio: the binary has
	// no subcommands and this feature does not introduce any.
	if target := visited["copy-store-to"]; target != "" {
		if cfg.Mode == "agent" {
			logger.Error("--copy-store-to is not accepted in agent mode",
				"fix", "an agent has no server data set to carry")
			os.Exit(copyExitAgentBad)
		}
		os.Exit(runCopy(cfg.DBPath, target, visited["yes"] == "true", os.Stdout, os.Stdin, logger))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.Mode == "agent" {
		dataDir := os.Getenv("MAINTENANT_DATA_DIR")
		if dataDir == "" {
			dataDir = defaultAgentDataDir
		}
		agentCfg := agent.AgentConfig{
			DataDir:            dataDir,
			ServerURL:          cfg.MultiHost.ServerURL,
			EnrollmentToken:    cfg.MultiHost.EnrollmentToken,
			RuntimeOverride:    cfg.MultiHost.RuntimeOverride,
			Label:              cfg.MultiHost.Label,
			AgentVersion:       version,
			InsecureSkipVerify: cfg.MultiHost.InsecureSkipVerify,
		}
		if err := agent.Run(ctx, agentCfg, logger); err != nil {
			logger.Error("agent run failed", "error", err)
			os.Exit(1)
		}
		return
	}

	application, err := app.New(cfg, logger)
	if err != nil {
		if !logStorageStartupError(logger, err, cfg.DatabaseURL) {
			logger.Error("failed to initialize application", "error", err)
		}
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
