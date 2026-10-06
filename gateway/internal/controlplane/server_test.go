package controlplane_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/primemcp/mcplake/internal/controlplane"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestServer starts a Server on an ephemeral loopback port and returns
// its bound address plus a cleanup func that gracefully stops it and asserts
// Start returned cleanly.
func startTestServer(t testing.TB) (addr string, cleanup func()) {
	t.Helper()

	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	startErr := make(chan error, 1)
	go func() { startErr <- s.Start(context.Background()) }()

	select {
	case <-s.Ready():
	case err := <-startErr:
		t.Fatalf("server failed to start: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server readiness")
	}

	cleanup = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))

		select {
		case err := <-startErr:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not return after Stop")
		}
	}
	return s.Addr(), cleanup
}

func TestServer_HealthzReturnsOKAfterStart(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/admin/healthz", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_UnknownRouteReturns404(t *testing.T) {
	addr, cleanup := startTestServer(t)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/admin/nope", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestServer_AdminGroupAcceptsAdditionalRoutes(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	s.Admin().GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	startErr := make(chan error, 1)
	go func() { startErr <- s.Start(context.Background()) }()
	select {
	case <-s.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server readiness")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, s.Stop(ctx))
		<-startErr
	}()

	resp, err := http.Get(fmt.Sprintf("http://%s/admin/ping", s.Addr()))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_StopWithAlreadyExpiredContextDoesNotHang(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	startErr := make(chan error, 1)
	go func() { startErr <- s.Start(context.Background()) }()

	select {
	case <-s.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server readiness")
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- s.Stop(ctx) }()

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

func TestServer_AddrEmptyBeforeStart(t *testing.T) {
	s := controlplane.NewServer(controlplane.Config{ControlPlaneAddr: "127.0.0.1:0"})
	assert.Empty(t, s.Addr())
}
