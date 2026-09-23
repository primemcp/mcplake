package cache

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"
)

// DefaultHealthCheckInterval is how often the health loop runs when the
// operator does not say otherwise.
//
// Unlike ADR-0013's schema refresh, which is opt-in because stale schemas
// are a correctness nicety with a real per-tick cost, this one defaults to
// on: without it an http or sse MCP is permanently broken by any restart of
// the server behind it, and "the gateway recovers from a downstream
// restart" is not a feature an operator should have to discover a config
// key to get. Thirty seconds bounds the worst-case outage after a restart
// while keeping the steady-state cost to one empty round trip per MCP per
// half minute.
const DefaultHealthCheckInterval = 30 * time.Second

// maxHealthBackoff caps the wait between reconnect attempts for an MCP that
// keeps failing. A downstream that is down for an afternoon should not be
// dialed 1,200 times, and a *misconfigured* stdio entry should not fork a
// doomed subprocess every tick — but the cap keeps recovery bounded, so an
// endpoint that comes back is picked up within five minutes however long it
// was away.
const maxHealthBackoff = 5 * time.Minute

// healthCheckable is the behavior HealthChecker needs from the Registry,
// satisfied by *Registry. Defined here so the loop's scheduling and backoff
// can be tested without live MCP clients.
type healthCheckable interface {
	Names() []string
	CheckHealth(ctx context.Context, name string) (HealthResult, bool)
}

// HealthChecker pings every registered MCP on a fixed interval and
// reconnects the ones that have stopped answering. It is the timer half of
// ADR-0019; the per-MCP work lives in Registry.CheckHealth.
//
// A downstream reached over http or sse has a lifecycle the gateway neither
// owns nor observes: it restarts, redeploys, gets rescheduled. Before this
// loop existed, the first such event left the registration holding a dead
// session and every call returning 502 until the gateway itself was
// restarted. This is the thing that notices.
type HealthChecker struct {
	registry healthCheckable
	interval time.Duration
	// backoff holds the consecutive-failure state for MCPs that could not
	// be reconnected. Only the Run goroutine touches it.
	backoff map[string]*healthBackoff
	// now is time.Now, replaceable in tests so backoff can be exercised
	// without sleeping through it.
	now func() time.Time
}

// healthBackoff is one MCP's consecutive-failure state.
type healthBackoff struct {
	failures int
	// retryAt is the earliest tick this MCP should be attempted again.
	retryAt time.Time
}

// NewHealthChecker returns a checker that sweeps reg every interval. A
// non-positive interval yields a checker whose Run is a no-op — callers
// gate construction on the configured interval being > 0, which is how an
// operator turns the loop off.
func NewHealthChecker(reg *Registry, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		registry: reg,
		interval: interval,
		backoff:  make(map[string]*healthBackoff),
		now:      time.Now,
	}
}

// Run sweeps on every tick until ctx is cancelled, then returns nil. A
// failing MCP never stops the loop, and one MCP's slow reconnect only
// delays the rest of that tick.
//
// ctx is passed straight through to CheckHealth, which reconnects with it —
// so it must be the gateway's lifetime context. See CheckHealth.
func (h *HealthChecker) Run(ctx context.Context) error {
	if h.interval <= 0 {
		slog.Debug("mcp health check disabled (interval <= 0)")
		return nil
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	slog.Info("mcp health check loop started", "interval", h.interval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			h.checkOnce(ctx)
		}
	}
}

func (h *HealthChecker) checkOnce(ctx context.Context) {
	names := h.registry.Names()
	h.forgetUnregistered(names)

	var healthy, recovered, failed, skipped int
	for _, name := range names {
		if ctx.Err() != nil {
			return
		}
		if h.skip(name) {
			skipped++
			continue
		}

		res, ok := h.registry.CheckHealth(ctx, name)
		if !ok {
			// Unregistered between the snapshot and now.
			delete(h.backoff, name)
			continue
		}
		switch {
		case res.Err != nil:
			failed++
			h.recordFailure(name, res.Err)
		case res.Reconnected:
			recovered++
			delete(h.backoff, name)
		default:
			healthy++
			delete(h.backoff, name)
		}
	}

	if recovered > 0 || failed > 0 {
		slog.Info("mcp health check tick",
			"healthy", healthy, "recovered", recovered, "failed", failed, "backing_off", skipped)
		return
	}
	slog.Debug("mcp health check tick",
		"healthy", healthy, "recovered", recovered, "failed", failed, "backing_off", skipped)
}

// skip reports whether name is still inside its backoff window.
func (h *HealthChecker) skip(name string) bool {
	state, ok := h.backoff[name]
	return ok && h.now().Before(state.retryAt)
}

// recordFailure extends name's backoff: the interval doubled once per
// consecutive failure, capped at maxHealthBackoff.
func (h *HealthChecker) recordFailure(name string, cause error) {
	state, ok := h.backoff[name]
	if !ok {
		state = &healthBackoff{}
		h.backoff[name] = state
	}
	state.failures++

	// Doubled once per consecutive failure, but stopped at the cap rather
	// than computed as interval<<failures: an MCP down for a week reaches
	// failures in the thousands, and that shift overflows into a negative
	// duration long before the loop below would run more than a handful of
	// times.
	delay := h.interval
	for range state.failures - 1 {
		if delay >= maxHealthBackoff {
			break
		}
		delay *= 2
	}
	delay = min(delay, maxHealthBackoff)
	state.retryAt = h.now().Add(delay)

	slog.Warn("mcp health check could not reconnect",
		"mcp", name, "consecutive_failures", state.failures, "retry_in", delay, "error", cause)
}

// forgetUnregistered drops backoff state for MCPs that are no longer
// registered, so the map cannot grow without bound across a long uptime of
// register/unregister churn — and so a name that is registered again starts
// from a clean slate rather than inheriting the old one's penalty.
func (h *HealthChecker) forgetUnregistered(names []string) {
	if len(h.backoff) == 0 {
		return
	}
	maps.DeleteFunc(h.backoff, func(name string, _ *healthBackoff) bool {
		return !slices.Contains(names, name)
	})
}
