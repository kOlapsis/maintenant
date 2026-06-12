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
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kolapsis/maintenant/internal/agent"
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

	// Multi-host flags (parsed before --mcp-stdio check so --mode is always available).
	fMode := flag.String("mode", "embedded", "operating mode: embedded|server|agent")
	fServer := flag.String("server", "", "gRPC server URL (agent mode)")
	fEnrollToken := flag.String("enrollment-token", "", "enrollment token (agent mode, first boot)")
	fRuntime := flag.String("runtime", "", "override runtime detection: docker|swarm|kubernetes")
	fLabel := flag.String("label", "", "display label for this agent (agent mode)")
	fGRPCListen := flag.String("grpc-listen", "", "override MAINTENANT_GRPC_LISTEN")
	fGRPCPublicURL := flag.String("grpc-url", "", "override MAINTENANT_GRPC_URL")
	fTLSCert := flag.String("grpc-tls-cert", "", "TLS certificate file (server mode)")
	fTLSKey := flag.String("grpc-tls-key", "", "TLS key file (server mode)")
	fInsecureSkip := flag.Bool("grpc-insecure-skip-tls-verify", false, "disable TLS verification (debug only)")
	fEmbeddedAgent := flag.Bool("embedded-agent", false, "also run a local agent (server mode, Pro)")

	// flag.Parse handles --foo and -foo; ignore unknown flags for --mcp-stdio compat.
	flag.CommandLine.SetOutput(os.Stderr)
	flag.Parse()

	logLevel := logLevelFromEnv()
	logOutput := os.Stdout
	if mcpStdio {
		logOutput = os.Stderr
	}
	logger := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{
		Level: logLevel,
	}))

	if *fInsecureSkip {
		logger.Warn("insecure TLS verification disabled — do not use in production")
	}

	logger.Info("maintenant starting", "version", version, "commit", commit, "build_date", buildDate, "mode", *fMode)

	cfg := app.ConfigFromEnv()
	cfg.Version = version
	cfg.Commit = commit
	cfg.BuildDate = buildDate
	cfg.PublicKeyB64 = publicKeyB64
	cfg.Mode = *fMode

	// CLI overrides for MultiHost config
	if *fGRPCListen != "" {
		cfg.MultiHost.GRPCListen = *fGRPCListen
	}
	if *fGRPCPublicURL != "" {
		cfg.MultiHost.GRPCPublicURL = *fGRPCPublicURL
	}
	cfg.MultiHost.TLSCertFile = *fTLSCert
	cfg.MultiHost.TLSKeyFile = *fTLSKey
	cfg.MultiHost.ServerURL = *fServer
	cfg.MultiHost.EnrollmentToken = *fEnrollToken
	cfg.MultiHost.RuntimeOverride = *fRuntime
	cfg.MultiHost.Label = *fLabel
	cfg.MultiHost.InsecureSkipVerify = *fInsecureSkip
	cfg.MultiHost.EmbeddedAgent = *fEmbeddedAgent

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// mode=agent: run enrollment then exit — no HTTP server needed.
	if cfg.Mode == "agent" {
		dataDir := os.Getenv("MAINTENANT_DATA_DIR")
		if dataDir == "" {
			dataDir = "/var/lib/maintenant"
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
