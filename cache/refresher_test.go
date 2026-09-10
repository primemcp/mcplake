package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingRefresher records how many times RefreshActive was called and
// returns a fixed result set.
type countingRefresher struct {
	calls   atomic.Int64
	results []RefreshResult
}

func (c *countingRefresher) RefreshActive(context.Context) []RefreshResult {
	c.calls.Add(1)
	return c.results
}

func TestSchemaRefresher_TicksAtIntervalUntilContextCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &countingRefresher{}
		s := &SchemaRefresher{registry: fake, interval: time.Minute}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- s.Run(ctx) }()

		// Advance three intervals; each should produce exactly one call.
		for want := int64(1); want <= 3; want++ {
			time.Sleep(time.Minute)
			synctest.Wait()
			assert.Equal(t, want, fake.calls.Load())
		}

		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			require.NoError(t, err)
		default:
			t.Fatal("Run did not return after context cancellation")
		}

		// No further ticks after return.
		before := fake.calls.Load()
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		assert.Equal(t, before, fake.calls.Load())
	})
}

func TestSchemaRefresher_FailureDoesNotStopTheLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &countingRefresher{results: []RefreshResult{
			{Name: "good", Err: nil},
			{Name: "bad", Err: errors.New("tools/list failed")},
		}}
		s := &SchemaRefresher{registry: fake, interval: time.Minute}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = s.Run(ctx) }()

		time.Sleep(3 * time.Minute)
		synctest.Wait()

		assert.GreaterOrEqual(t, fake.calls.Load(), int64(3), "the loop keeps ticking despite a per-MCP failure")
	})
}

func TestSchemaRefresher_NonPositiveIntervalIsNoOp(t *testing.T) {
	fake := &countingRefresher{}
	s := &SchemaRefresher{registry: fake, interval: 0}

	require.NoError(t, s.Run(context.Background()))
	assert.Zero(t, fake.calls.Load())
}

func TestNewSchemaRefresher_UsesRegistry(t *testing.T) {
	r := NewRegistry()
	s := NewSchemaRefresher(r, time.Minute)
	assert.Equal(t, time.Minute, s.interval)
	assert.NotNil(t, s.registry)
}
