// Package internal implements the mcplake gateway's data-plane HTTP server.
//
// See docs/architecture/decisions/0001-use-fasthttp-for-gateway-server.rst for
// why this is built on fasthttp instead of net/http, and
// docs/architecture/data.rst#request-lifecycle for the full tool-call pipeline
// this server will eventually orchestrate.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/primemcp/mcplake/auth"
	"github.com/primemcp/mcplake/cache"
	"github.com/valyala/fasthttp"
)

// idleTimeout bounds how long a keep-alive connection may sit idle. fasthttp
// documents that Shutdown/ShutdownWithContext deliberately does not close
// keepalive connections itself, so without an IdleTimeout a client holding a
// persistent connection open (as most HTTP clients do by default) could keep
// graceful shutdown from ever completing.
const idleTimeout = 60 * time.Second

// defaultCallTimeout bounds a single downstream tool call when
// Config.CallTimeout is unset.
const defaultCallTimeout = 30 * time.Second

// Authenticator verifies a caller's bearer token. Satisfied by
// *auth.Validator; defined here (consumer-side) so it can be substituted
// with a fake in tests.
type Authenticator interface {
	ValidateToken(ctx context.Context, token string) (*auth.Claims, error)
}

// PolicyEngine answers the two questions the pipeline needs per call:
// whether it's authorized, and which response fields to strip. Satisfied by
// *router.Engine.
type PolicyEngine interface {
	Authorize(claims json.RawMessage, mcp, tool string) (bool, error)
	FieldsToRemove(claims json.RawMessage, mcp, tool string) ([]string, error)
}

// MCPResolver resolves a registered MCP name to a live client, answers
// whether a given tool exists on it, and reports whether it is
// administratively disabled. Satisfied by *cache.Registry.
type MCPResolver interface {
	Resolve(mcp string) (cache.MCPClient, bool)
	HasTool(mcp, tool string) bool
	// Disabled reports whether mcp is registered but turned off by an
	// operator (see #95). A disabled MCP stays connected with its tools
	// cached; the pipeline rejects calls to it without a downstream call.
	Disabled(mcp string) bool
	// List returns every registration the registry holds, with its status,
	// operator flag and discovered tool schemas. It is what the data-plane
	// MCP endpoint builds its tools/list from (ADR-0021); the REST
	// POST /v1/call path, which is told the mcp and tool up front, has no
	// need of it.
	List() []cache.MCPRegistration
	// Registered reports whether mcp is known to the registry at all,
	// whatever its status. It is what separates "no such MCP" (404) from
	// "registered, currently unreachable, being reconnected" (503) once
	// Resolve has declined to hand over a client — see ADR-0019.
	Registered(mcp string) bool
}

// Config configures the data-plane Gateway.
type Config struct {
	// DataPlaneAddr is the listen address (host:port) for the tool-call
	// proxy, e.g. ":8080". See config.ServerConfig.DataPlaneAddr.
	DataPlaneAddr string

	// Authenticator, Policy, and Resolver implement the auth -> authorize ->
	// route stages of the pipeline (docs/architecture/data.rst#request-lifecycle).
	// A POST /v1/call whose request parses successfully but which reaches a
	// stage with a nil dependency here fails closed with 501, rather than
	// panicking — see handleToolCall.
	Authenticator Authenticator
	Policy        PolicyEngine
	Resolver      MCPResolver

	// CallTimeout bounds a single downstream tool call, independent of the
	// inbound request's own lifecycle. Defaults to 30s.
	CallTimeout time.Duration
}

// Gateway is the fasthttp-based data-plane server: it terminates tool-call
// requests and orchestrates auth -> authorize -> route -> call -> filter.
//
// Gateway must not be copied after first use.
type Gateway struct {
	addr string

	authenticator Authenticator
	policy        PolicyEngine
	resolver      MCPResolver
	callTimeout   time.Duration

	server *fasthttp.Server

	// baseCtx is the gateway's own lifetime, cancelled by Stop. It is the
	// root of a streamable MCP session's context, which must outlive the
	// request that created it; see mcpSessionScope.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	// mcpStreamable and mcpSSE serve the data plane's MCP endpoints
	// (ADR-0021). Built once by buildMCPHandlers.
	mcpStreamable fasthttp.RequestHandler
	mcpSSE        fasthttp.RequestHandler

	mu    sync.Mutex
	ln    net.Listener
	ready chan struct{}
	once  sync.Once
}

// NewGateway constructs a Gateway bound to cfg.DataPlaneAddr. It does not
// start listening; call Start to do that.
func NewGateway(cfg Config) *Gateway {
	callTimeout := cfg.CallTimeout
	if callTimeout <= 0 {
		callTimeout = defaultCallTimeout
	}

	baseCtx, baseCancel := context.WithCancel(context.Background())
	g := &Gateway{
		addr:          cfg.DataPlaneAddr,
		authenticator: cfg.Authenticator,
		policy:        cfg.Policy,
		resolver:      cfg.Resolver,
		callTimeout:   callTimeout,
		baseCtx:       baseCtx,
		baseCancel:    baseCancel,
		ready:         make(chan struct{}),
	}
	g.server = &fasthttp.Server{
		Handler:     g.handleRequest,
		IdleTimeout: idleTimeout,
	}
	g.buildMCPHandlers()
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
	// Cancelled before Shutdown, not after, and deliberately not deferred.
	// An open MCP session holds a hanging GET, and fasthttp's graceful
	// shutdown waits for in-flight requests to finish -- so ending the
	// sessions is what lets Shutdown return rather than something to tidy
	// up once it has. Streamable sessions hold a context rooted here; SSE
	// sessions hold their request's, which fasthttp closes as the server
	// shuts down.
	g.baseCancel()
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
		g.handleToolCall(ctx)
	// Both MCP handlers are method-agnostic: the transports key off the
	// method and a session header or query parameter, not off the path, so
	// one case per path covers the whole transport.
	case path == MCPStreamablePath:
		g.mcpStreamable(ctx)
	case path == MCPSSEPath:
		g.mcpSSE(ctx)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}
