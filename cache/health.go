package cache

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/atsokha/mcplake/mcp"
)

// healthProbeTimeout bounds the `ping` a health check sends to an active
// MCP. The failure this whole file exists for is a peer that is no longer
// answering, and half of those still accept a TCP connection — so an
// unbounded probe is itself a way to hang the health loop on exactly the
// MCP it was meant to notice. It is deliberately short: a healthy server
// answers an empty round trip immediately, and a slow one gets another
// chance on the next tick.
const healthProbeTimeout = 5 * time.Second

// healthProbeAttempts is how many times a ping is tried before the session
// is treated as dead.
//
// One failed ping does not mean the session is gone. The most common cause
// is a *pooled TCP connection* that the server closed and this client had
// not yet noticed: the gateway's shared http transport keeps an idle
// connection for 90s (see mcp.httpTransportClient), while a server behind
// uvicorn or Node closes one after 5, so the first request after a quiet
// spell can pick a corpse out of the pool and fail with EOF. The MCP
// session on the server is fine; only the socket died.
//
// A second ping dials afresh (Go's transport evicts the dead connection on
// the error) and carries the same session id, so it succeeds — where a
// server that has actually restarted fails again. Telling those apart costs
// one round trip on the failure path and saves rebuilding a working
// session, which in turn saves a full tools/list.
const healthProbeAttempts = 2

// HealthResult is the outcome of checking one MCP.
//
// Status is what the registration holds *after* the check, so a caller can
// log the transition without reading the Registry again. Err is non-nil
// exactly when the MCP is not usable afterwards; it carries why the
// reconnect failed, which is the part an operator needs.
type HealthResult struct {
	Name   string
	Status string
	// Reconnected is true when this check replaced a client: either it
	// revived an unreachable registration, or it found an active one's
	// session dead and rebuilt it.
	Reconnected bool
	Err         error
}

// Names returns every currently registered name, sorted.
//
// It exists for the health loop, which needs to decide per MCP whether this
// tick is one it should attempt (see HealthChecker's backoff) and so cannot
// use a bulk call. List would also answer the question, but it deep-copies
// every registration's tool map to do it — once per tick, forever, to read
// the keys.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Sorted(maps.Keys(r.byName))
}

// CheckHealth verifies that name's MCP is reachable and repairs it if it is
// not, returning what it found and whether name is registered at all.
//
// The three cases:
//
//   - Active with a live session: `ping` answers, nothing happens. This is
//     the overwhelmingly common one and costs an empty round trip.
//   - Active with a dead session: the ping fails, and a reconnect is
//     attempted immediately. If it succeeds the registration never leaves
//     StatusActive — the client and tools are swapped underneath it, so a
//     concurrent caller sees the old working client or the new one, never a
//     gap.
//   - Not active: a reconnect is attempted. This is how a registration that
//     failed at startup (an MCP not yet listening, an address not yet
//     resolving) becomes usable once its server arrives, without an
//     operator re-POSTing it.
//
// When the reconnect fails the registration is demoted to
// StatusUnreachable and its dead client closed, so Resolve stops handing
// out a connection that cannot carry a call. The cached tool schemas are
// deliberately kept: Resolve already gates on Status, so a stale list
// cannot be called, and dropping it would empty the admin UI's
// response-filter editor for the duration of an outage.
//
// ctx must be the gateway's lifetime context, not a per-tick or
// per-request one. A reconnect over stdio spawns a subprocess with
// exec.CommandContext, so a short-lived ctx would kill the MCP moments
// after reviving it. The bounded waits that a health check does need are
// derived here (healthProbeTimeout) or enforced by the http transport's own
// per-phase timeouts.
//
// See ADR-0019.
func (r *Registry) CheckHealth(ctx context.Context, name string) (HealthResult, bool) {
	r.mu.RLock()
	reg, ok := r.byName[name]
	r.mu.RUnlock()
	if !ok {
		return HealthResult{}, false
	}

	if reg.Status == StatusActive && reg.Client != nil {
		if err := probe(ctx, reg.Client); err != nil {
			slog.Warn("mcp session is not answering; reconnecting",
				"mcp", name, "transport", reg.Transport, "error", err)
		} else {
			return HealthResult{Name: name, Status: StatusActive}, true
		}
	}

	// Shutdown is not a downstream failure. Once ctx is done every dial
	// fails instantly, and reconnecting here would demote every MCP and log
	// a warning per registration on the way out -- an alarming last page of
	// logs describing nothing that happened.
	if ctx.Err() != nil {
		return HealthResult{Name: name, Status: reg.Status}, true
	}

	if err := r.reconnect(ctx, reg); err != nil {
		return HealthResult{Name: name, Status: StatusUnreachable, Err: err}, true
	}
	return HealthResult{Name: name, Status: StatusActive, Reconnected: true}, true
}

// probe pings client, retrying once so a dead pooled connection is not
// mistaken for a dead session. It returns the last error, which is the one
// that actually justifies a reconnect.
func probe(ctx context.Context, client MCPClient) error {
	var err error
	for attempt := range healthProbeAttempts {
		if attempt > 0 && ctx.Err() != nil {
			return err
		}
		probeCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
		err = client.Ping(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
	}
	return err
}

// CheckAll checks every registration, returning one result each. One MCP's
// failure never stops the others. The set is snapshotted up front; an MCP
// registered mid-run is picked up on the next call.
//
// The health loop uses Names + CheckHealth directly so it can apply its
// per-MCP backoff; this is the unconditional form, for callers (and tests)
// that want a single sweep.
func (r *Registry) CheckAll(ctx context.Context) []HealthResult {
	names := r.Names()
	results := make([]HealthResult, 0, len(names))
	for _, name := range names {
		if res, ok := r.CheckHealth(ctx, name); ok {
			results = append(results, res)
		}
	}
	return results
}

// reconnect builds a fresh client for reg from its stored transport and
// connect config, re-discovers its tools, and swaps both into the stored
// registration.
//
// It is make-before-break, for the same reason Register is (ADR-0003): the
// new client is live and its tools discovered before anything shared is
// touched, so a reconnect that fails leaves a working registration exactly
// as it was, and one that succeeds is a single atomic map write.
//
// reg is the snapshot the caller decided to act on. Anything that replaced
// it in the meantime — an admin re-registering the same name against a
// different URL, an Unregister — wins: the swap is skipped and the client
// built here is closed. Identity is the client pointer, the same test
// RefreshTools uses.
func (r *Registry) reconnect(ctx context.Context, reg MCPRegistration) error {
	client, err := newMCPClient(ctx, mcp.Config{
		Transport: reg.Transport,
		Command:   reg.Connect.Command,
		Arguments: reg.Connect.Arguments,
		Env:       reg.Connect.Env,
		URL:       reg.Connect.URL,
	})
	if err != nil {
		r.demote(reg)
		return fmt.Errorf("cache: reconnect to %q: %w", reg.Name, err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		r.demote(reg)
		return fmt.Errorf("cache: rediscover tools for %q: %w", reg.Name, err)
	}

	toolMap := toolMapFrom(tools)

	superseded, swapped := r.activate(reg, client, toolMap)
	if !swapped {
		// The registration moved on under us; the client just built has no
		// owner and would otherwise leak a connection (or a subprocess).
		_ = client.Close()
		return nil
	}
	if superseded != nil {
		_ = superseded.Close()
	}
	slog.Info("mcp reconnected", "mcp", reg.Name, "transport", reg.Transport, "tools", len(toolMap))
	return nil
}

// activate swaps client and tools into the entry reg was read from, marking
// it StatusActive, and returns the client it replaced (nil if there was
// none) plus whether the swap happened at all.
//
// Only Status, Client and Tools are written. Connect, Transport and Enabled
// are left as currently stored rather than restored from reg, so a
// concurrent SetEnabled is not reverted by a reconnect that started before
// it.
func (r *Registry) activate(reg MCPRegistration, client MCPClient, tools map[string]ToolSchema) (superseded MCPClient, swapped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.byName[reg.Name]
	if !ok || current.Client != reg.Client {
		return nil, false
	}
	superseded = current.Client
	current.Status = StatusActive
	current.Client = client
	current.Tools = tools
	r.byName[reg.Name] = current
	return superseded, true
}

// demote records that reg's MCP could not be reached: StatusUnreachable,
// no client, and the dead one closed.
//
// Tools are kept on purpose — see CheckHealth. Status is what makes a
// registration uncallable (Resolve and HasTool both gate on it), so keeping
// the schemas costs nothing in safety and keeps the admin UI able to show
// and edit filters for an endpoint that is merely down.
//
// Like activate, it is a no-op if the registration has since been replaced.
func (r *Registry) demote(reg MCPRegistration) {
	r.mu.Lock()
	current, ok := r.byName[reg.Name]
	if !ok || current.Client != reg.Client {
		r.mu.Unlock()
		return
	}
	dead := current.Client
	wasActive := current.Status == StatusActive
	current.Status = StatusUnreachable
	current.Client = nil
	r.byName[reg.Name] = current
	r.mu.Unlock()

	if dead != nil {
		_ = dead.Close()
	}
	if wasActive {
		slog.Warn("mcp is no longer reachable", "mcp", reg.Name, "transport", reg.Transport)
	}
}
