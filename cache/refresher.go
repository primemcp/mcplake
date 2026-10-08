package cache

import (
	"context"
	"log/slog"
	"time"
)

// activeRefresher is the behavior SchemaRefresher needs from the Registry,
// satisfied by *Registry. Defined here so the loop can be tested without a
// real registry full of live MCP clients.
type activeRefresher interface {
	RefreshActive(ctx context.Context) []RefreshResult
}

// SchemaRefresher re-discovers every active MCP's tool schema on a fixed
// interval by calling Registry.RefreshActive. It is the timer half of
// ADR-0013; the per-MCP work and its failure handling live in
// Registry.RefreshTools.
type SchemaRefresher struct {
	registry activeRefresher
	interval time.Duration
}

// NewSchemaRefresher returns a refresher that calls reg.RefreshActive every
// interval. A non-positive interval yields a refresher whose Run is a no-op
// — callers gate construction on the configured interval being > 0.
func NewSchemaRefresher(reg *Registry, interval time.Duration) *SchemaRefresher {
	return &SchemaRefresher{registry: reg, interval: interval}
}

// Run refreshes on every tick until ctx is cancelled, then returns nil. Each
// tick logs a one-line summary at debug and one WARN per MCP that failed; a
// refresh failure never stops the loop. Run is a no-op if the interval is
// non-positive.
func (s *SchemaRefresher) Run(ctx context.Context) error {
	if s.interval <= 0 {
		slog.Debug("mcp schema refresh disabled (interval <= 0)")
		return nil
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	slog.Info("mcp schema refresh loop started", "interval", s.interval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.refreshOnce(ctx)
		}
	}
}

func (s *SchemaRefresher) refreshOnce(ctx context.Context) {
	results := s.registry.RefreshActive(ctx)
	failed := 0
	for _, res := range results {
		if res.Err != nil {
			failed++
			slog.Warn("mcp schema refresh failed", "mcp", res.Name, "error", res.Err)
		}
	}
	slog.Debug("mcp schema refresh tick",
		"refreshed", len(results)-failed, "failed", failed, "total", len(results))
}
