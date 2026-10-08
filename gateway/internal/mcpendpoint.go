package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/auth"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// The data plane's MCP endpoints. See ADR-0021.
const (
	// MCPStreamablePath serves the MCP Streamable HTTP transport, the
	// current standard. A client POSTs JSON-RPC to it and may hold a GET
	// open for server-initiated messages.
	MCPStreamablePath = "/v1/mcp"
	// MCPSSEPath serves the older HTTP+SSE transport: a hanging GET opens
	// the session and carries responses, and the client POSTs messages back
	// to the same path with the session id the stream handed it. Kept
	// because plenty of deployed clients still speak only this, the same
	// reasoning ADR-0017 applied on the downstream side.
	MCPSSEPath = "/v1/sse"
)

// mcpKeepAlive is how often the gateway pings a connected MCP client.
//
// It is not a nicety. fasthttp's RequestCtx.Done() is closed only when the
// *server* shuts down -- it is the server's channel, not the request's,
// because allocating one per request is too expensive. So neither transport
// learns from its context that a client hung up, and an abandoned SSE
// session would otherwise sit in SSEHandler.ServeHTTP until the gateway
// stops. A keepalive ping fails against a vanished peer, and the sdk closes
// the session on that failure, which is what reaps it.
const mcpKeepAlive = 30 * time.Second

// mcpInstructions is handed to a connecting client during initialize.
const mcpInstructions = "Tools are served by mcplake, a gateway in front of one or more downstream MCP " +
	"servers. Each tool is named <mcp>__<tool>, after the registered MCP it belongs to. " +
	"The catalogue is filtered to what your credentials authorize, so it may differ between callers " +
	"and may change as an operator registers, disables or removes a downstream."

// claimsContextKey types the request-context slot the MCP endpoint's auth
// wrapper puts validated claims in, for getServer to read back.
type claimsContextKey struct{}

// buildMCPHandlers constructs the two adapted handlers once, at
// construction time, so a request does not pay for building them.
func (g *Gateway) buildMCPHandlers() {
	streamable := sdk.NewStreamableHTTPHandler(g.mcpServerForRequest, nil)
	sse := sdk.NewSSEHandler(g.mcpServerForRequest, nil)

	g.mcpStreamable = g.adaptMCPHandler(streamable, mcpTransportStreamable)
	g.mcpSSE = g.adaptMCPHandler(sse, mcpTransportSSE)
}

// mcpTransportKind distinguishes the two endpoints. They differ in more than
// their wire format: which context a session may hold, and whether a request
// may go unauthenticated, both follow from it.
type mcpTransportKind int

const (
	// mcpTransportStreamable is MCP Streamable HTTP, served at
	// MCPStreamablePath.
	//
	// Its sessions get a context rooted at the gateway's lifetime rather
	// than at the request's: connectStreamable stores the request's context
	// in a ServerSession that outlives the POST that created it, and under
	// fasthttpadaptor that context is the *fasthttp.RequestCtx -- which
	// fasthttp resets and hands to an unrelated connection the moment the
	// handler returns. Retaining it would race a live session's context
	// reads against another request's reset.
	mcpTransportStreamable mcpTransportKind = iota
	// mcpTransportSSE is the older HTTP+SSE transport, served at MCPSSEPath.
	//
	// Its sessions keep the request's own context: SSEHandler.ServeHTTP
	// blocks on req.Context().Done() for the session's entire life, so
	// fasthttp keeps the RequestCtx alive throughout (it is released only
	// after the response body stream completes), and that channel closing on
	// server shutdown is what lets the handler return at all.
	mcpTransportSSE
)

// adaptMCPHandler wraps one of the sdk's net/http handlers for the fasthttp
// data plane: authenticate, bind the claims a session will run under, put
// the session on the right context, and adapt.
//
// The adaptor is what keeps this a route rather than a second listener.
// fasthttp v1.73's fasthttpadaptor watches for the first Flush and switches
// from buffering the response to SetBodyStreamWriter, so a handler that
// streams -- which both of these do -- streams through it.
func (g *Gateway) adaptMCPHandler(h http.Handler, kind mcpTransportKind) fasthttp.RequestHandler {
	return fasthttpadaptor.NewFastHTTPHandler(g.authenticateMCP(h, kind))
}

// authenticateMCP is the MCP endpoints' front door.
//
// Every request that carries an Authorization header has it validated, and
// one is required to establish a session. The exception is an SSE message
// POST, which names an already-established session in its query string: the
// sdk's own SSE client does not resend the header on those, so requiring it
// would reject a conforming client. Such a request is authorized by
// possession of the unguessable session id that an authenticated GET was
// handed. See ADR-0021.
func (g *Gateway) authenticateMCP(next http.Handler, kind mcpTransportKind) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.authenticator == nil {
			writeMCPHTTPError(w, http.StatusNotImplemented, "not_implemented", "tool-call pipeline is not configured")
			return
		}

		token, err := bearerTokenFrom(r)
		if err != nil {
			if !mcpTokenOptional(r, kind) {
				writeMCPHTTPError(w, http.StatusUnauthorized, "missing_authorization", err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(g.mcpSessionContext(r, kind, nil)))
			return
		}

		claims, err := g.authenticator.ValidateToken(r.Context(), token)
		if err != nil {
			writeMCPHTTPError(w, http.StatusUnauthorized, "unauthorized", "token failed validation")
			return
		}
		next.ServeHTTP(w, r.WithContext(g.mcpSessionContext(r, kind, claims)))
	})
}

// mcpSessionContext builds the context the sdk handler -- and any session it
// creates -- will run under, carrying claims for mcpServerForRequest to read
// back. See mcpTransportKind for why the two transports differ.
func (g *Gateway) mcpSessionContext(r *http.Request, kind mcpTransportKind, claims *auth.Claims) context.Context {
	parent := r.Context()
	if kind == mcpTransportStreamable {
		parent = g.baseCtx
	}
	if claims == nil {
		return parent
	}
	return context.WithValue(parent, claimsContextKey{}, claims)
}

// mcpTokenOptional reports whether a missing Authorization header is
// acceptable on r: only for an SSE message POST, which names an
// already-established session in its query string.
//
// The transport check is not decoration. The streamable transport keys its
// sessions off an Mcp-Session-Id *header*, so if this exemption applied
// there, a POST carrying that header plus any `?sessionid=` at all would
// skip token validation entirely.
func mcpTokenOptional(r *http.Request, kind mcpTransportKind) bool {
	return kind == mcpTransportSSE &&
		r.Method == http.MethodPost &&
		r.URL.Query().Get("sessionid") != ""
}

// bearerTokenFrom reads and validates the shape of r's Authorization header.
// It mirrors extractBearerToken, which reads the same header off fasthttp's
// own request type on the POST /v1/call path.
func bearerTokenFrom(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", unauthorized("missing_authorization", "Authorization header is required")
	}
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", unauthorized("invalid_authorization", "Authorization header must use the Bearer scheme")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	if token == "" {
		return "", unauthorized("invalid_authorization", "bearer token is empty")
	}
	return token, nil
}

// writeMCPHTTPError writes the same {"error","message"} body POST /v1/call
// uses, so a caller sees one error shape from the data plane whichever way
// it arrived.
func writeMCPHTTPError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

// mcpServerForRequest builds the MCP server a new session runs against,
// bound to the claims the auth wrapper validated for the request that
// established it.
//
// The sdk populates RequestExtra.Header -- per-request headers a handler
// could read -- only on the streamable transport; sse.go never sets it, and
// the sdk's SSE client does not resend Authorization on message POSTs. So
// there is no mechanism that would give both transports a fresh token per
// JSON-RPC message, and identity is bound once, here.
//
// What is *not* bound here is authorization: every tools/list and every
// tools/call re-runs Authorize against the live policy engine, so revoking a
// grant takes effect on the caller's next request rather than on its next
// connection.
func (g *Gateway) mcpServerForRequest(r *http.Request) *sdk.Server {
	claims, _ := r.Context().Value(claimsContextKey{}).(*auth.Claims)
	if claims == nil {
		// Only reachable for an SSE message POST with no header, which
		// cannot create a session anyway -- the sdk looks the session up by
		// id and never consults getServer. Returning nil makes the sdk
		// answer 400 rather than serving an unauthenticated session.
		return nil
	}

	server := sdk.NewServer(&sdk.Implementation{Name: "mcplake-gateway", Version: "0.1.0"}, &sdk.ServerOptions{
		Instructions: mcpInstructions,
		KeepAlive:    mcpKeepAlive,
		// No tool is registered with AddTool -- the catalogue is whatever
		// the registry holds and this caller may reach, resolved per
		// request by the middleware below -- so the tools capability has to
		// be declared rather than inferred.
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
	})
	server.AddReceivingMiddleware(g.mcpToolMiddleware(claims))
	return server
}

// mcpToolMiddleware answers tools/list and tools/call from the registry and
// the policy engine.
//
// It is middleware rather than AddTool because the tool set is not known at
// construction time and is not the same for two callers: it is the live
// registry filtered by this caller's grants. Everything else -- initialize,
// ping, the notification plumbing -- falls through to the sdk.
func (g *Gateway) mcpToolMiddleware(claims *auth.Claims) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			switch method {
			case "tools/list":
				return g.MCPListTools(ctx, claims)
			case "tools/call":
				// CallToolRequest is ServerRequest[*CallToolParamsRaw] --
				// the server side of a tool call carries raw arguments,
				// not the decoded CallToolParams a client sends.
				params, ok := req.GetParams().(*sdk.CallToolParamsRaw)
				if !ok {
					return next(ctx, method, req)
				}
				return g.MCPCallTool(ctx, claims, params)
			default:
				return next(ctx, method, req)
			}
		}
	}
}
