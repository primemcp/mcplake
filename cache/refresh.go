package cache

import (
	"context"
	"fmt"
)

// RefreshResult is the outcome of refreshing one MCP's tool schema. Err is
// nil on success (including the no-op cases in RefreshTools).
type RefreshResult struct {
	Name string
	Err  error
}

// RefreshTools re-discovers name's tools by calling tools/list on its
// existing live client — no reconnect — and atomically swaps the fresh
// schema into the Registry, preserving Status, Client, Enabled, Connect and
// Transport. See ADR-0013.
//
// It is a no-op (returns nil) when name is absent, not StatusActive, or has
// no client. A tools/list failure returns a wrapped error and leaves the
// stored registration completely untouched — a transient blip must not
// evict a working MCP. If the registration was removed or its client
// replaced (by Unregister or a concurrent Register) between the discovery
// call and the swap, the stale result is discarded.
func (r *Registry) RefreshTools(ctx context.Context, name string) error {
	r.mu.RLock()
	reg, ok := r.byName[name]
	r.mu.RUnlock()
	if !ok || reg.Status != StatusActive || reg.Client == nil {
		return nil
	}

	tools, err := reg.Client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("cache: refresh tools for %q: %w", name, err)
	}

	toolMap := toolMapFrom(tools)

	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.byName[name]
	if !ok || current.Client != reg.Client || current.Status != StatusActive {
		return nil
	}
	current.Tools = toolMap
	r.byName[name] = current
	return nil
}

// RefreshActive refreshes every registration that is currently StatusActive
// with a live client, returning one RefreshResult per MCP. One MCP's
// failure never stops the others. The set of MCPs to refresh is snapshotted
// up front; an MCP registered mid-run is picked up on the next call.
func (r *Registry) RefreshActive(ctx context.Context) []RefreshResult {
	r.mu.RLock()
	names := make([]string, 0, len(r.byName))
	for name, reg := range r.byName {
		if reg.Status == StatusActive && reg.Client != nil {
			names = append(names, name)
		}
	}
	r.mu.RUnlock()

	results := make([]RefreshResult, 0, len(names))
	for _, name := range names {
		results = append(results, RefreshResult{Name: name, Err: r.RefreshTools(ctx, name)})
	}
	return results
}
