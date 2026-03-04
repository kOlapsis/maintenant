package oauth

import (
	"context"
	"log/slog"
	"time"
)

// StartCleanup runs periodic cleanup of expired codes and tokens.
// It blocks until ctx is cancelled.
func StartCleanup(ctx context.Context, store MCPOAuthStore, logger *slog.Logger) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := store.DeleteExpired(ctx)
			if err != nil {
				logger.Error("mcp oauth cleanup failed", "error", err)
			} else if deleted > 0 {
				logger.Info("mcp oauth cleanup", "deleted", deleted)
			}
		}
	}
}
