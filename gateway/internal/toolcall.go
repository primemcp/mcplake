package internal

import (
	"encoding/json"
	"errors"
	"strings"

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

// handleToolCall implements POST /v1/call's request parsing and validation.
// The pipeline behind a successfully-parsed request (auth -> authorize ->
// route -> call -> filter) is wired in by #9; until then, a well-formed
// request still receives 501, echoing back what was parsed.
func (g *Gateway) handleToolCall(ctx *fasthttp.RequestCtx) {
	req, err := parseToolCallRequest(ctx)
	if err != nil {
		var reqErr *requestError
		if errors.As(err, &reqErr) {
			writeError(ctx, reqErr.status, reqErr.code, reqErr.message)
			return
		}
		writeError(ctx, fasthttp.StatusInternalServerError, "internal_error", "unexpected error parsing request")
		return
	}

	writeJSON(ctx, fasthttp.StatusNotImplemented, map[string]any{
		"error":   "not_implemented",
		"message": "tool-call pipeline not yet wired",
		"mcp":     req.MCP,
		"tool":    req.Tool,
	})
}
