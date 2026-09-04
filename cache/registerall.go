package cache

import (
	"context"
	"log/slog"
)

// RegisterAll registers every given MCPRegistration, tolerating individual
// failures: one unreachable MCP is logged and skipped, it does not prevent
// the others from registering, and registration order does not matter — a
// failure anywhere in the list never short-circuits the rest. This is what
// lets gateway startup proceed with a partially-available set of MCPs
// rather than failing outright. See ADR-0003.
func (r *Registry) RegisterAll(ctx context.Context, regs []MCPRegistration) {
	for _, reg := range regs {
		if err := r.Register(ctx, reg); err != nil {
			slog.Warn("mcp registration failed", "mcp", reg.Name, "error", err)
		}
	}
}
