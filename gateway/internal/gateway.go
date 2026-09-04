// Package internal implements the mcplake gateway's data-plane HTTP server.
//
// See docs/architecture/decisions/0001-use-fasthttp-for-gateway-server.md for
// why this is built on fasthttp instead of net/http, and
// docs/architecture/data.md#request-lifecycle for the full tool-call pipeline
// this server will eventually orchestrate.
package internal

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/valyala/fasthttp"
)

// Config configures the data-plane Gateway.
type Config struct {
	// DataPlaneAddr is the listen address (host:port) for the tool-call
	// proxy, e.g. ":8080". See config.ServerConfig.DataPlaneAddr.
	DataPlaneAddr string
}

// Gateway is the fasthttp-based data-plane server: it terminates tool-call
// requests and (once the JWT policy pipeline and MCP registry are wired in by
// later tickets) orchestrates auth -> authorize -> route -> call -> filter.
//
// Gateway must not be copied after first use.
type Gateway struct {
	addr   string
	server *fasthttp.Server

	mu    sync.Mutex
	ln    net.Listener
	ready chan struct{}
	once  sync.Once
}

// NewGateway constructs a Gateway bound to cfg.DataPlaneAddr. It does not
// start listening; call Start to do that.
func NewGateway(cfg Config) *Gateway {
	g := &Gateway{
		addr:  cfg.DataPlaneAddr,
		ready: make(chan struct{}),
	}
	g.server = &fasthttp.Server{
		Handler: g.handleRequest,
	}
	return g
}

// Ready returns a channel that is closed once the Gateway has bound its
// listener and is accepting connections.
func (g *Gateway) Ready() <-chan struct{} {
	return g.ready
}

// Addr returns the actual bound address, useful when Config.DataPlaneAddr
// used an ephemeral port (":0") such as in tests. It is empty until Start has
// bound the listener.
func (g *Gateway) Addr() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ln == nil {
		return ""
	}
	return g.ln.Addr().String()
}

// Start binds the listener and serves requests until the server is shut down
// via Stop, in which case Start returns nil, or an unrecoverable server error
// occurs.
func (g *Gateway) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", g.addr)
	if err != nil {
		return fmt.Errorf("gateway: listen on %s: %w", g.addr, err)
	}

	g.mu.Lock()
	g.ln = ln
	g.mu.Unlock()
	g.once.Do(func() { close(g.ready) })

	if err := g.server.Serve(ln); err != nil {
		return fmt.Errorf("gateway: serve: %w", err)
	}
	return nil
}

// Stop gracefully shuts the server down: it stops accepting new connections
// and waits for in-flight requests to finish, up to ctx's deadline. It
// unblocks the goroutine running Start.
func (g *Gateway) Stop(ctx context.Context) error {
	if err := g.server.ShutdownWithContext(ctx); err != nil {
		return fmt.Errorf("gateway: shutdown: %w", err)
	}
	return nil
}

// handleRequest is the data-plane's entire route table. It intentionally uses
// a manual method+path switch rather than a router dependency; see ADR-0001.
func (g *Gateway) handleRequest(ctx *fasthttp.RequestCtx) {
	path := string(ctx.Path())
	method := string(ctx.Method())

	switch {
	case method == fasthttp.MethodGet && path == "/healthz":
		ctx.SetStatusCode(fasthttp.StatusOK)
		ctx.SetContentType("text/plain; charset=utf-8")
		ctx.SetBodyString("ok")
	case method == fasthttp.MethodPost && path == "/v1/call":
		// Stubbed pending the request-parsing (#8) and pipeline-wiring (#9)
		// tickets. Deliberately not a 404: the route exists, the pipeline
		// behind it doesn't yet.
		ctx.SetStatusCode(fasthttp.StatusNotImplemented)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(`{"error":"not_implemented","message":"tool-call pipeline not yet wired"}`)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}
