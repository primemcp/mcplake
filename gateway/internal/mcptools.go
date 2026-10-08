package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/auth"
	"github.com/primemcp/mcplake/cache"
)

// toolNameSeparator joins a registered MCP's name to one of its tools to
// form the name this gateway advertises. A caller sees
// "postgres-ro__get_user", not "get_user": every registered MCP's catalogue
// is flattened into one list, and two downstreams may well both advertise a
// "query" or a "search".
//
// The separator is not unambiguous -- an MCP name or a tool name may contain
// it -- so splitting a qualified name back apart resolves against the
// registry rather than parsing. See splitToolName.
const toolNameSeparator = "__"

// emptyObjectSchema stands in for a tool whose MCP advertises no input
// schema. `inputSchema` is required by the protocol, and a client handed
// `null` there has nothing to build a call from.
var emptyObjectSchema = json.RawMessage(`{"type":"object"}`)

// qualifyToolName is the name this gateway advertises for one downstream
// tool.
func qualifyToolName(mcpName, tool string) string {
	return mcpName + toolNameSeparator + tool
}

// MCPListTools answers tools/call's sibling: every (mcp, tool) pair this
// caller's access policy grants, across every registration that is active
// and enabled, as MCP tools.
//
// Filtering the catalogue by the same Authorize decision that gates the call
// is the point. It makes the access policy discoverable rather than merely
// enforceable -- two callers with different grants see different lists --
// and it keeps a caller from being offered a tool whose only possible answer
// is 403.
//
// It is exported for the endpoint's tests; callers outside this package
// reach it through the MCP endpoint, not directly.
func (g *Gateway) MCPListTools(_ context.Context, claims *auth.Claims) (*sdk.ListToolsResult, error) {
	if g.policy == nil || g.resolver == nil {
		return nil, errors.New("tool-call pipeline is not configured")
	}

	regs := g.resolver.List()
	// Sorted by MCP name, then tool name: a client's tool list should not
	// reshuffle between calls just because the registry is a map.
	slices.SortFunc(regs, func(a, b cache.MCPRegistration) int {
		return strings.Compare(a.Name, b.Name)
	})

	tools := make([]*sdk.Tool, 0, len(regs))
	for _, reg := range regs {
		// The same two gates the pipeline applies, applied to discovery: a
		// disabled MCP rejects every call (#95) and an unreachable one has
		// no live session to call (ADR-0019), so neither belongs in a
		// catalogue a caller is meant to act on.
		if !reg.Enabled || reg.Status != cache.StatusActive {
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(reg.Tools)) {
			authorized, err := g.policy.Authorize(claims.Raw, reg.Name, name)
			if err != nil {
				return nil, fmt.Errorf("policy evaluation failed for %q: %w", qualifyToolName(reg.Name, name), err)
			}
			if !authorized {
				continue
			}
			tools = append(tools, toolFor(reg.Name, reg.Tools[name]))
		}
	}
	return &sdk.ListToolsResult{Tools: tools}, nil
}

// toolFor converts one cached downstream schema into the sdk.Tool this
// gateway advertises for it.
func toolFor(mcpName string, schema cache.ToolSchema) *sdk.Tool {
	tool := &sdk.Tool{
		Name:        qualifyToolName(mcpName, schema.Name),
		Description: schema.Description,
		InputSchema: emptyObjectSchema,
	}
	if len(schema.InputSchema) > 0 {
		tool.InputSchema = schema.InputSchema
	}
	// Assigned only when present: sdk.Tool.OutputSchema is an `any`, so a
	// nil json.RawMessage stored in it is still a non-nil interface and
	// would marshal to `null` rather than being omitted.
	if len(schema.OutputSchema) > 0 {
		tool.OutputSchema = schema.OutputSchema
	}
	return tool
}

// MCPCallTool runs a tools/call through the gateway's one and only
// pipeline -- auth -> authorize -> route -> call -> filter -- and returns the
// downstream's result with every drop_fields path already removed.
//
// Failures split two ways, following the MCP spec's distinction between a
// call that could not be made and a call that failed:
//
//   - A protocol error (a returned error) for anything decided before the
//     downstream was reached: an unknown tool, a missing grant, a disabled
//     MCP. The call should not have happened, and a model gains nothing by
//     reformulating it.
//   - A result with IsError set for a downstream that was reached and
//     failed, timed out, or answered something the filter could not redact.
//     The call was legitimate; the model can react to it.
//
// It is exported for the endpoint's tests; callers outside this package
// reach it through the MCP endpoint, not directly.
//
// params is the sdk's server-side shape (CallToolRequest is
// ServerRequest[*CallToolParamsRaw]): arguments arrive as raw JSON, because
// only a tool's own handler knows its schema. Here that handler is a
// downstream MCP, so they are decoded to a map and passed on.
func (g *Gateway) MCPCallTool(ctx context.Context, claims *auth.Claims, params *sdk.CallToolParamsRaw) (*sdk.CallToolResult, error) {
	if g.policy == nil || g.resolver == nil {
		return nil, errors.New("tool-call pipeline is not configured")
	}

	mcpName, tool, ok := g.splitToolName(params.Name)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", params.Name)
	}

	args, err := toolArguments(params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool %q: %w", params.Name, err)
	}

	// The same detachment POST /v1/call performs, and for the same reason:
	// a downstream call gets its own deadline rather than inheriting a
	// transport's, which for a streamable session is the gateway's whole
	// lifetime.
	callCtx, cancel := context.WithTimeout(ctx, g.callTimeout)
	defer cancel()

	body, err := g.callWithClaims(callCtx, claims, mcpName, tool, args)
	if err != nil {
		return mcpCallFailure(params.Name, err)
	}

	var result sdk.CallToolResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("tool %q: decode filtered result: %w", params.Name, err)
	}
	return &result, nil
}

// mcpCallFailure maps a pipeline error onto the two channels a tools/call
// has. See MCPCallTool for which failures go where.
func mcpCallFailure(name string, err error) (*sdk.CallToolResult, error) {
	var reqErr *requestError
	if !errors.As(err, &reqErr) {
		return nil, fmt.Errorf("tool %q: %w", name, err)
	}
	switch reqErr.code {
	case "forbidden", "mcp_disabled", "mcp_not_found", "tool_not_found", "unauthorized", "not_implemented":
		return nil, fmt.Errorf("tool %q: %s: %s", name, reqErr.code, reqErr.message)
	default:
		return &sdk.CallToolResult{
			IsError: true,
			Content: []sdk.Content{
				&sdk.TextContent{Text: fmt.Sprintf("%s: %s", reqErr.code, reqErr.message)},
			},
		}, nil
	}
}

// toolArguments decodes the raw arguments into the map the pipeline passes
// downstream. A client that sends none at all is calling a no-argument tool,
// not making a mistake.
func toolArguments(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("arguments are not a JSON object: %w", err)
	}
	return decoded, nil
}

// splitToolName resolves a qualified tool name back to the registration and
// tool it names.
//
// It resolves rather than parses. Splitting on the first or the last
// separator would silently mis-route as soon as an MCP name or a tool name
// contains one -- "odd__name__do__it" has three plausible readings and only
// the registry knows which is real. Where more than one registration could
// claim the name, the longest MCP name wins, since that is the more specific
// reading.
func (g *Gateway) splitToolName(qualified string) (mcpName, tool string, ok bool) {
	for _, reg := range g.resolver.List() {
		rest, found := strings.CutPrefix(qualified, reg.Name+toolNameSeparator)
		if !found {
			continue
		}
		if _, has := reg.Tools[rest]; !has {
			continue
		}
		if len(reg.Name) > len(mcpName) {
			mcpName, tool, ok = reg.Name, rest, true
		}
	}
	return mcpName, tool, ok
}
