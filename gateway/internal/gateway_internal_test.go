package internal

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

// TestGateway_StopDrainsInFlightRequest is a white-box test (same package) so
// it can inject an artificially slow handler to create a real in-flight
// request window, which the public routes (/healthz, /v1/call) can't do
// since they both respond instantly.
func TestGateway_StopDrainsInFlightRequest(t *testing.T) {
	g := NewGateway(Config{DataPlaneAddr: "127.0.0.1:0"})

	requestStarted := make(chan struct{})
	const handlerDelay = 300 * time.Millisecond
	g.server.Handler = func(ctx *fasthttp.RequestCtx) {
		close(requestStarted)
		time.Sleep(handlerDelay)
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetBodyString("slow-ok")
	}

	startErr := make(chan error, 1)
	go func() { startErr <- g.Start(context.Background()) }()

	select {
	case <-g.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gateway readiness")
	}

	addr := g.Addr()

	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			errCh <- err
			return
		}
		respCh <- resp
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request never reached the handler")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopErr := make(chan error, 1)
	go func() { stopErr <- g.Stop(stopCtx) }()

	select {
	case resp := <-respCh:
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	case err := <-errCh:
		t.Fatalf("in-flight request failed instead of draining: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request never completed")
	}

	require.NoError(t, <-stopErr)
	require.NoError(t, <-startErr)
}

// TestGateway_HandleRequestRouting exercises the route switch directly
// against a fasthttp.RequestCtx, without going over the network.
//
// POST /v1/call's own status-code branches (400/401/501) are covered in
// toolcall_test.go; this table only checks that requests reach the right
// handler at all.
func TestGateway_HandleRequestRouting(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "healthz", method: fasthttp.MethodGet, path: "/healthz", wantStatus: fasthttp.StatusOK},
		{name: "unknown path", method: fasthttp.MethodGet, path: "/nope", wantStatus: fasthttp.StatusNotFound},
		{name: "wrong method on healthz", method: fasthttp.MethodPost, path: "/healthz", wantStatus: fasthttp.StatusNotFound},
	}

	g := NewGateway(Config{DataPlaneAddr: "127.0.0.1:0"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx fasthttp.RequestCtx
			ctx.Request.Header.SetMethod(tt.method)
			ctx.Request.SetRequestURI(tt.path)

			g.handleRequest(&ctx)

			assert.Equal(t, tt.wantStatus, ctx.Response.StatusCode())
		})
	}
}
