package internal_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/primemcp/mcplake/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestGateway starts a Gateway on an ephemeral loopback port and returns
// its bound address plus a cleanup func that gracefully stops it and asserts
// Start returned cleanly. Takes testing.TB (not *testing.T) so it doubles as
// setup for benchmarks (ticket #11), not just tests.
func startTestGateway(t testing.TB) (addr string, cleanup func()) {
	t.Helper()
	return startTestGatewayWithConfig(t, internal.Config{})
}

// startTestGatewayWithConfig is startTestGateway but lets the caller supply
// pipeline dependencies (Authenticator/Policy/Resolver/CallTimeout);
// DataPlaneAddr is always overridden to an ephemeral loopback port.
func startTestGatewayWithConfig(t testing.TB, cfg internal.Config) (addr string, cleanup func()) {
	t.Helper()

	cfg.DataPlaneAddr = "127.0.0.1:0"
	g := internal.NewGateway(cfg)
	startErr := make(chan error, 1)
	go func() { startErr <- g.Start(context.Background()) }()

	select {
	case <-g.Ready():
	case err := <-startErr:
		t.Fatalf("gateway failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gateway readiness")
	}

	cleanup = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, g.Stop(ctx))

		select {
		case err := <-startErr:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return after Stop")
		}
	}
	return g.Addr(), cleanup
}

func TestGateway_HealthzReturnsOKAfterStart(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGateway_UnknownPathReturns404(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/nope", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGateway_WrongMethodOnHealthzReturns404(t *testing.T) {
	addr, cleanup := startTestGateway(t)
	defer cleanup()

	resp, err := http.Post(fmt.Sprintf("http://%s/healthz", addr), "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGateway_StopWithAlreadyExpiredContextDoesNotHang(t *testing.T) {
	g := internal.NewGateway(internal.Config{DataPlaneAddr: "127.0.0.1:0"})
	startErr := make(chan error, 1)
	go func() { startErr <- g.Start(context.Background()) }()

	select {
	case <-g.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gateway readiness")
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- g.Stop(ctx) }()

	select {
	case <-done:
		// Either a nil (no open connections) or a deadline-exceeded-flavored
		// error is acceptable; hanging is not.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop hung with an already-expired context")
	}

	select {
	case <-startErr:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestGateway_AddrEmptyBeforeStart(t *testing.T) {
	g := internal.NewGateway(internal.Config{DataPlaneAddr: "127.0.0.1:0"})
	assert.Empty(t, g.Addr())
}
