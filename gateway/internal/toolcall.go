package internal

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/atsokha/mcplake/filter"
	"github.com/valyala/fasthttp"
)

// ToolCallRequest is the fully-parsed, RequestCtx-detached representation of
// a POST /v1/call request. Ticket #9 consumes this through the auth ->
// authorize -> route -> call -> filter pipeline.
type ToolCallRequest struct {
	MCP       string
	Tool      string
	Arguments map[string]any
	// Token is the raw bearer JWT, not yet validated.
	Token string
}

// requestError is a parsing/validation failure that maps directly to an HTTP
// status and a machine-readable error code.
type requestError struct {
	status  int
	code    string
	message string
}

func (e *requestError) Error() string { return e.message }

func badRequest(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusBadRequest, code: code, message: message}
}

func unauthorized(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusUnauthorized, code: code, message: message}
}

func forbidden(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusForbidden, code: code, message: message}
}

func notFound(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusNotFound, code: code, message: message}
}

func badGateway(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusBadGateway, code: code, message: message}
}

func serviceUnavailable(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusServiceUnavailable, code: code, message: message}
}

func gatewayTimeout(code, message string) *requestError {
	return &requestError{status: fasthttp.StatusGatewayTimeout, code: code, message: message}
}

func internalError(message string) *requestError {
	return &requestError{status: fasthttp.StatusInternalServerError, code: "internal_error", message: message}
}

// toolCallBody mirrors the wire shape of a POST /v1/call request body.
type toolCallBody struct {
	MCP       string         `json:"mcp"`
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// parseToolCallRequest extracts and validates a ToolCallRequest from ctx.
//
// Every returned field is a value already copied out of ctx's buffers by
// encoding/json (which allocates fresh strings/maps when unmarshaling) or by
// fasthttp's header accessors (which return copies, not sub-slices of the
// connection buffer for this use). The result is therefore safe to use after
// the handler returns, when fasthttp has already reset and possibly reused
// ctx for a different connection.
func parseToolCallRequest(ctx *fasthttp.RequestCtx) (ToolCallRequest, error) {
	var body toolCallBody
	if err := json.Unmarshal(ctx.PostBody(), &body); err != nil {
		return ToolCallRequest{}, badRequest("invalid_json", "request body is not valid JSON")
	}
	if body.MCP == "" {
		return ToolCallRequest{}, badRequest("missing_field", "\"mcp\" is required")
	}
	if body.Tool == "" {
		return ToolCallRequest{}, badRequest("missing_field", "\"tool\" is required")
	}

	token, err := extractBearerToken(ctx)
	if err != nil {
		return ToolCallRequest{}, err
	}

	return ToolCallRequest{
		MCP:       body.MCP,
		Tool:      body.Tool,
		Arguments: body.Arguments,
		Token:     token,
	}, nil
}

const bearerPrefix = "Bearer "

// extractBearerToken reads and validates the Authorization header, returning
// a plain Go string detached from ctx.
func extractBearerToken(ctx *fasthttp.RequestCtx) (string, error) {
	// Peek returns a slice backed by ctx's connection buffer; converting to
	// string here copies it, so the result outlives the handler safely.
	header := string(ctx.Request.Header.Peek("Authorization"))
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

// writeError writes a structured JSON error response.
func writeError(ctx *fasthttp.RequestCtx, status int, code, message string) {
	writeJSON(ctx, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

// writeJSON marshals v and writes it as the response body. Marshaling
// failures fall back to a 500 with a fixed body, since v is always an
// internally-constructed value in this package.
func writeJSON(ctx *fasthttp.RequestCtx, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetContentType("application/json")
		ctx.SetBodyString(`{"error":"internal_error","message":"failed to encode response"}`)
		return
	}
	ctx.SetStatusCode(status)
	ctx.SetContentType("application/json")
	ctx.SetBody(body)
}

// handleToolCall implements POST /v1/call end to end: parse -> auth ->
// authorize -> route -> call -> filter. See
// docs/architecture/data.md#request-lifecycle for the sequence this
// mirrors.
func (g *Gateway) handleToolCall(ctx *fasthttp.RequestCtx) {
	req, err := parseToolCallRequest(ctx)
	if err != nil {
		writeRequestError(ctx, err)
		return
	}

	// Detach from fasthttp's request lifecycle: RequestCtx is pooled and
	// reused once this handler returns, so everything from here on runs
	// under a fresh context with its own explicit deadline (see ADR-0001),
	// not one derived from ctx.
	callCtx, cancel := context.WithTimeout(context.Background(), g.callTimeout)
	defer cancel()

	body, err := g.runPipeline(callCtx, req)
	if err != nil {
		writeRequestError(ctx, err)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	ctx.SetBody(body)
}

// writeRequestError maps err to its HTTP response if it's a *requestError,
// or falls back to a generic 500 for anything else.
func writeRequestError(ctx *fasthttp.RequestCtx, err error) {
	var reqErr *requestError
	if errors.As(err, &reqErr) {
		writeError(ctx, reqErr.status, reqErr.code, reqErr.message)
		return
	}
	writeError(ctx, fasthttp.StatusInternalServerError, "internal_error", "unexpected error")
}

// runPipeline executes auth -> authorize -> route -> call -> filter for a
// successfully-parsed request, returning the filtered response body. Every
// error it returns is a *requestError, already mapped to the right status.
func (g *Gateway) runPipeline(ctx context.Context, req ToolCallRequest) (json.RawMessage, error) {
	if g.authenticator == nil || g.policy == nil || g.resolver == nil {
		// Fails closed rather than nil-panicking when the gateway is
		// constructed without the pipeline wired in (e.g. by an older
		// caller, or a test that only cares about /healthz).
		return nil, &requestError{
			status:  fasthttp.StatusNotImplemented,
			code:    "not_implemented",
			message: "tool-call pipeline is not configured",
		}
	}

	claims, err := g.authenticator.ValidateToken(ctx, req.Token)
	if err != nil {
		return nil, unauthorized("unauthorized", "token failed validation")
	}

	authorized, err := g.policy.Authorize(claims.Raw, req.MCP, req.Tool)
	if err != nil {
		return nil, internalError("policy evaluation failed")
	}
	if !authorized {
		return nil, forbidden("forbidden", "not authorized to call this tool")
	}

	// An administratively disabled MCP is a routing-stage rejection, kept
	// distinct from an unknown one: the registration and its client still
	// exist, an operator has just turned it off (see #95). Checked after
	// authorization so its state is not disclosed to unauthorized callers.
	if g.resolver.Disabled(req.MCP) {
		return nil, forbidden("mcp_disabled", "mcp is disabled")
	}

	client, ok := g.resolver.Resolve(req.MCP)
	if !ok {
		// Registered but not active is a different answer from unknown, and
		// since ADR-0019 it is a *recoverable* one: the health loop is
		// already trying to reconnect it, so the caller should retry rather
		// than conclude the MCP does not exist. Conflating the two under
		// 404 sent an operator looking for a missing registration when the
		// registration was fine and the downstream was merely down.
		if g.resolver.Registered(req.MCP) {
			return nil, serviceUnavailable("mcp_unavailable", "mcp is registered but not currently reachable")
		}
		return nil, notFound("mcp_not_found", "mcp not found")
	}
	if !g.resolver.HasTool(req.MCP, req.Tool) {
		return nil, notFound("tool_not_found", "tool not found on this mcp")
	}

	resp, err := client.CallTool(ctx, req.Tool, req.Arguments)
	if err != nil {
		if ctx.Err() != nil {
			return nil, gatewayTimeout("upstream_timeout", "downstream mcp call timed out")
		}
		return nil, badGateway("upstream_error", "downstream mcp call failed")
	}

	fields, err := g.policy.FieldsToRemove(claims.Raw, req.MCP, req.Tool)
	if err != nil {
		return nil, internalError("policy evaluation failed")
	}

	filtered, removed, err := filter.StripToolResult(resp.Raw, fields)
	if err != nil {
		if errors.Is(err, filter.ErrUnenforceable) {
			// The operator asked for fields to be removed and this
			// response is shaped so that they cannot be. Returning it
			// would hand over exactly what they meant to withhold, so
			// the call fails instead. See ADR-0015.
			slog.Error("response filtering could not be enforced; refusing to return the response",
				"mcp", req.MCP, "tool", req.Tool, "fields", len(fields), "error", err)
			return nil, badGateway("filter_unenforceable", "response filtering could not be enforced")
		}
		return nil, internalError("response filtering failed")
	}
	if len(fields) > 0 && removed == 0 {
		// A filter policy matched this call and then removed nothing.
		// Almost always a path that no longer matches the tool's shape
		// (renamed field, renamed tool) -- silent until now.
		slog.Warn("filter policy matched but removed no fields",
			"mcp", req.MCP, "tool", req.Tool, "fields", len(fields))
	}
	return filtered, nil
}
