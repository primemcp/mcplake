package cache

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedRegistry is a healthCheckable that records which names each tick
// asked about and answers from a per-name script.
type scriptedRegistry struct {
	mu sync.Mutex
	// names is what Names returns; a test can change it mid-run.
	names []string
	// results maps a name to the outcome CheckHealth should report. A name
	// absent from the map is reported as healthy.
	results map[string]HealthResult
	// checked accumulates every name CheckHealth was called with, in order.
	checked []string
}

func newScriptedRegistry(names ...string) *scriptedRegistry {
	return &scriptedRegistry{names: names, results: make(map[string]HealthResult)}
}

func (s *scriptedRegistry) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.names)
}

func (s *scriptedRegistry) CheckHealth(_ context.Context, name string) (HealthResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Contains(s.names, name) {
		return HealthResult{}, false
	}
	s.checked = append(s.checked, name)
	if res, ok := s.results[name]; ok {
		res.Name = name
		return res, true
	}
	return HealthResult{Name: name, Status: StatusActive}, true
}

func (s *scriptedRegistry) setResult(name string, res HealthResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[name] = res
}

func (s *scriptedRegistry) setNames(names ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = names
}

// countChecks returns how many times name was checked.
func (s *scriptedRegistry) countChecks(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, checked := range s.checked {
		if checked == name {
			n++
		}
	}
	return n
}

// newTestChecker wires a HealthChecker to reg without going through
// NewHealthChecker, which takes a concrete *Registry.
func newTestChecker(reg healthCheckable, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		registry: reg,
		interval: interval,
		backoff:  make(map[string]*healthBackoff),
		now:      time.Now,
	}
}

func TestHealthChecker_ChecksEveryMCPOnEveryTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("a", "b")
		h := newTestChecker(reg, time.Minute)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- h.Run(ctx) }()

		for want := 1; want <= 3; want++ {
			time.Sleep(time.Minute)
			synctest.Wait()
			assert.Equal(t, want, reg.countChecks("a"))
			assert.Equal(t, want, reg.countChecks("b"))
		}

		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			require.NoError(t, err)
		default:
			t.Fatal("Run did not return after context cancellation")
		}
	})
}

func TestHealthChecker_NonPositiveIntervalNeverRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("a")
		h := newTestChecker(reg, 0)

		require.NoError(t, h.Run(t.Context()), "Run must return immediately, not block")

		time.Sleep(time.Hour)
		synctest.Wait()
		assert.Zero(t, reg.countChecks("a"))
	})
}

// A failing MCP must not stall the ones after it in the sweep, and must not
// stop the loop.
func TestHealthChecker_FailureDoesNotStopTheLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("broken", "fine")
		reg.setResult("broken", HealthResult{Status: StatusUnreachable, Err: errors.New("refused")})
		h := newTestChecker(reg, time.Minute)

		go func() { _ = h.Run(t.Context()) }()

		time.Sleep(3 * time.Minute)
		synctest.Wait()
		assert.Equal(t, 3, reg.countChecks("fine"))
	})
}

// A permanently unreachable MCP backs off: the interval, then double it,
// and so on. Over ten ticks it should be attempted at 1, 2, 4 and 8 rather
// than all ten.
func TestHealthChecker_FailingMCPBacksOffExponentially(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("down")
		reg.setResult("down", HealthResult{Status: StatusUnreachable, Err: errors.New("refused")})
		h := newTestChecker(reg, time.Minute)

		go func() { _ = h.Run(t.Context()) }()

		want := map[int]int{1: 1, 2: 2, 3: 2, 4: 3, 5: 3, 6: 3, 7: 3, 8: 4}
		for tick := 1; tick <= 8; tick++ {
			time.Sleep(time.Minute)
			synctest.Wait()
			assert.Equal(t, want[tick], reg.countChecks("down"),
				"attempts after %d ticks", tick)
		}
	})
}

func TestHealthChecker_BackoffIsCapped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("down")
		reg.setResult("down", HealthResult{Status: StatusUnreachable, Err: errors.New("refused")})
		h := newTestChecker(reg, time.Minute)

		go func() { _ = h.Run(t.Context()) }()

		// Long enough for an uncapped doubling to have pushed the next
		// attempt years out.
		time.Sleep(time.Hour)
		synctest.Wait()
		before := reg.countChecks("down")

		// At the cap, one more attempt must land within maxHealthBackoff
		// plus a tick's slack.
		time.Sleep(maxHealthBackoff + time.Minute)
		synctest.Wait()
		assert.Greater(t, reg.countChecks("down"), before,
			"a capped backoff must keep retrying, so an MCP that returns is picked up")
	})
}

// Recovery has to clear the penalty, or an MCP that was down for an hour
// keeps being checked at five-minute intervals long after it came back.
func TestHealthChecker_SuccessResetsBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("flaky")
		reg.setResult("flaky", HealthResult{Status: StatusUnreachable, Err: errors.New("refused")})
		h := newTestChecker(reg, time.Minute)

		go func() { _ = h.Run(t.Context()) }()

		// Fail enough to build up a multi-tick backoff.
		time.Sleep(10 * time.Minute)
		synctest.Wait()

		reg.setResult("flaky", HealthResult{Status: StatusActive, Reconnected: true})
		// One attempt lands, recovers, and clears the penalty...
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		recovered := reg.countChecks("flaky")

		// ...so from here it is checked every single tick again.
		time.Sleep(3 * time.Minute)
		synctest.Wait()
		assert.Equal(t, recovered+3, reg.countChecks("flaky"))
	})
}

// Backoff state is keyed by name, so an unregistered-then-reregistered MCP
// must not inherit the old one's penalty (and the map must not grow across
// a long uptime of churn).
func TestHealthChecker_ForgetsBackoffForUnregisteredMCPs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reg := newScriptedRegistry("gone")
		reg.setResult("gone", HealthResult{Status: StatusUnreachable, Err: errors.New("refused")})
		h := newTestChecker(reg, time.Minute)

		go func() { _ = h.Run(t.Context()) }()

		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.NotEmpty(t, h.backoff)

		reg.setNames()
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Empty(t, h.backoff, "backoff state for a removed MCP must not be retained")

		// Registered again under the same name: no inherited penalty, so it
		// is attempted on the very next tick.
		reg.setNames("gone")
		reg.setResult("gone", HealthResult{Status: StatusActive, Reconnected: true})
		before := reg.countChecks("gone")
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, before+1, reg.countChecks("gone"))
	})
}
