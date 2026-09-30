// Package mcp implements a client connection to a single downstream MCP
// server, built on the official github.com/modelcontextprotocol/go-sdk.
// See docs/architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.rst.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config describes how to reach one downstream MCP.
//
// Which fields matter depends on Transport: stdio uses Command/Arguments/
// Env, http and sse use URL. NewClient rejects a Config that omits what
// its transport needs, rather than dialing something half-specified.
type Config struct {
	// Transport is one of the Transport* constants. Empty means
	// TransportStdio -- every caller predating the other two passed a
	// Command and nothing else, and they should keep working unchanged.
	Transport string
	// Command and Arguments are the subprocess to spawn, for stdio.
	Command   string
	Arguments []string
	// Env is additional environment for the stdio subprocess, beyond the
	// documented base set (see baseSubprocessEnv). A downstream MCP is
	// trusted to serve tools, not with the gateway's own secrets, so it
	// does not inherit the gateway's full environment -- an MCP that
	// genuinely needs a credential gets it explicitly, here. A key that
	// collides with the base set overrides it: the operator named it on
	// purpose. Ignored for http/sse, which run as a separate process (or
	// no process at all) that this gateway never spawns.
	Env map[string]string
	// URL is the server's endpoint, for http and sse. It must satisfy
	// ValidateEndpointURL.
	URL string
}

// transport resolves Transport's zero value to stdio.
func (c Config) transport() string {
	if c.Transport == "" {
		return TransportStdio
	}
	return c.Transport
}

// ToolSchema describes one tool a downstream MCP advertises. It mirrors
// cache.ToolSchema; the two aren't the same type because mcp must not
// depend on cache (cache, the Registry, depends on mcp — not the reverse).
type ToolSchema struct {
	Name string
	// Description is the tool's human-readable description, as the MCP
	// advertised it. It is what an MCP client shows a model to let it pick
	// a tool, so the data-plane MCP endpoint (ADR-0021) carries it through
	// to its own tools/list. Empty when the MCP doesn't advertise one.
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage // nil when the MCP doesn't advertise one
}

// ToolResponse is a tool call's raw JSON-RPC result. What to do with it
// (which fields to keep, how to interpret Content vs. StructuredContent) is
// the Policy Engine and Response Filter's job (ADR-0004), not this
// package's — Raw is deliberately the whole marshaled result.
type ToolResponse struct {
	Raw     json.RawMessage
	IsError bool
}

// Client is a live connection to one downstream MCP server.
//
// Client must not be copied after first use.
type Client struct {
	session *sdk.ClientSession
}

// NewClient connects to the MCP described by cfg and performs the
// initialize handshake.
//
// For stdio the subprocess's lifetime is tied to ctx (via
// exec.CommandContext): callers that want a long-lived connection (as the
// Registry does once it holds a Client for a registration) must pass a
// context that outlives this call, not a short-lived request context. The
// same applies to http and sse, where ctx bounds the session's underlying
// requests rather than a process.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	transport, target, err := newTransport(ctx, cfg)
	if err != nil {
		return nil, err
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "mcplake-gateway", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect to %q: %w", target, err)
	}
	return &Client{session: session}, nil
}

// newTransport builds the sdk transport cfg names, and returns it with a
// human-readable target for error messages (the command, or the URL).
func newTransport(ctx context.Context, cfg Config) (sdk.Transport, string, error) {
	switch t := cfg.transport(); t {
	case TransportStdio:
		if cfg.Command == "" {
			return nil, "", fmt.Errorf("mcp: Config.Command is required for the %s transport", TransportStdio)
		}
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Arguments...)
		cmd.Env = subprocessEnv(cfg.Env)
		return &sdk.CommandTransport{Command: cmd}, cfg.Command, nil

	case TransportHTTP:
		if err := validateURLFor(t, cfg.URL); err != nil {
			return nil, "", err
		}
		return &sdk.StreamableClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpTransportClient,
			// The standalone SSE stream is the transport's optional
			// second channel: a GET the client holds open for the
			// lifetime of the session so the server can push
			// notifications (tools/list_changed and friends) unprompted.
			//
			// This gateway consumes none of them. ADR-0013 settled on
			// polling tools/list on an interval instead, precisely so
			// schema freshness does not depend on a downstream
			// implementing notifications. Leaving the stream on would
			// therefore hold one connection open per registered MCP
			// forever, plus a reconnect loop behind it, and buy nothing.
			//
			// Turn this back on if the gateway ever starts reacting to
			// server-initiated messages -- at which point ADR-0013's
			// polling is what should be reconsidered, not this flag on
			// its own.
			DisableStandaloneSSE: true,
		}, cfg.URL, nil

	case TransportSSE:
		if err := validateURLFor(t, cfg.URL); err != nil {
			return nil, "", err
		}
		return &sdk.SSEClientTransport{
			Endpoint:   cfg.URL,
			HTTPClient: httpTransportClient,
		}, cfg.URL, nil

	default:
		return nil, "", fmt.Errorf("mcp: unsupported transport %q (supported: %s)",
			t, strings.Join(SupportedTransports(), ", "))
	}
}

func validateURLFor(transport, raw string) error {
	if raw == "" {
		return fmt.Errorf("mcp: Config.URL is required for the %s transport", transport)
	}
	if err := ValidateEndpointURL(raw); err != nil {
		return fmt.Errorf("mcp: Config.URL: %w", err)
	}
	return nil
}

// baseSubprocessEnvVars is the minimal, documented set of the gateway's own
// environment variables an MCP subprocess inherits by default -- just
// enough for it to run and resolve its own tooling, not to see anything
// about the gateway it's running under. See docs/CONFIG.md's `env` field
// for the operator-facing statement of this list; keep the two in sync.
//
//   - PATH    -- to resolve interpreters/binaries it shells out to itself
//     (bunx, node, python, ...). Without it, a stdio MCP launched via a
//     wrapper script commonly can't find anything to wrap.
//   - HOME    -- tools that cache or configure under the user's home
//     directory (npm/bun/pip caches, credential helpers) need it to agree
//     with PATH-resolved tooling about where that is.
//   - LANG, LC_ALL -- locale, so a subprocess's own text output/parsing
//     isn't silently different from the gateway's.
//   - TZ      -- so timestamps a subprocess generates agree with the
//     gateway's, without it needing to be told separately.
//   - TMPDIR  -- so temp files it creates land on a filesystem/permission
//     set that's actually known to work, matching the gateway's own.
var baseSubprocessEnvVars = []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ", "TMPDIR"}

// subprocessEnv builds the environment for an MCP subprocess: the
// documented base set (whichever of baseSubprocessEnvVars the gateway's
// own environment actually has set), overlaid with the MCP's own declared
// Env -- explicit beats inherited, for exactly the keys the operator
// named. Everything else in the gateway's environment (secrets very much
// included) is absent; this is what closes off exec.Cmd's documented
// default of inheriting the entire parent environment when Env is left
// nil.
func subprocessEnv(declared map[string]string) []string {
	env := make([]string, 0, len(baseSubprocessEnvVars)+len(declared))
	for _, key := range baseSubprocessEnvVars {
		if _, overridden := declared[key]; overridden {
			continue // the declared value wins; added below.
		}
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	for key, value := range declared {
		env = append(env, key+"="+value)
	}
	return env
}

// ListTools returns every tool the MCP currently advertises, transparently
// paging through the server's tools/list results.
func (c *Client) ListTools(ctx context.Context) ([]ToolSchema, error) {
	var schemas []ToolSchema
	for tool, err := range c.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp: list tools: %w", err)
		}

		input, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal input schema for tool %q: %w", tool.Name, err)
		}

		var output json.RawMessage
		if tool.OutputSchema != nil {
			output, err = json.Marshal(tool.OutputSchema)
			if err != nil {
				return nil, fmt.Errorf("mcp: marshal output schema for tool %q: %w", tool.Name, err)
			}
		}

		schemas = append(schemas, ToolSchema{
			Name:         tool.Name,
			Description:  tool.Description,
			InputSchema:  input,
			OutputSchema: output,
		})
	}
	return schemas, nil
}

// Ping issues an MCP `ping` on the existing session and reports whether the
// server answered.
//
// It exists to answer one question the Registry cannot otherwise ask: is
// this session still usable? For stdio that is nearly always yes, because
// the subprocess is the gateway's own child. For http and sse it is a real
// question — the server on the other end is somebody else's process, free
// to restart, redeploy or be rescheduled, and nothing tells this client
// when that happens. See ADR-0019.
//
// `ping` rather than `tools/list` because liveness and schema freshness are
// separate concerns with separate costs: ping is an empty round trip, while
// tools/list may page through a large catalogue and is already ADR-0013's
// job on its own interval.
//
// A server that replies "method not found" counts as healthy — see
// answeredButUnimplemented.
//
// Callers should bound ctx. A peer that accepted the connection and then
// stopped answering is exactly the case this has to detect, and without a
// deadline it is also the case that hangs here.
func (c *Client) Ping(ctx context.Context) error {
	err := c.session.Ping(ctx, nil)
	if err == nil || answeredButUnimplemented(err) {
		return nil
	}
	return fmt.Errorf("mcp: ping: %w", err)
}

// answeredButUnimplemented reports whether err is the server telling us it
// does not implement `ping` — which, for a liveness probe, is a *success*.
//
// `ping` is in the MCP spec and the official SDKs answer it, but plenty of
// deployed servers do not, and they say so with a JSON-RPC
// -32601 "Method not found" response. That response is itself the
// strongest liveness signal available: the request reached the server, the
// server decoded it, matched it against its method table and replied on
// this session. Treating it as a dead session made the health loop tear the
// session down and rebuild it on every single tick, forever, against a
// server that was working perfectly — churning `initialize` + `tools/list`
// at the health-check interval and eventually tipping a slow downstream
// into "unreachable" when one of those rebuilds failed. See #194.
//
// Only this one code is forgiven. Any other JSON-RPC error to a ping is
// ambiguous (a server answering -32603 to an empty request is not obviously
// well), and every transport-level failure still means what it meant: no
// answer came back, so the session is gone.
func answeredButUnimplemented(err error) bool {
	wire, ok := errors.AsType[*jsonrpc.Error](err)
	return ok && wire.Code == jsonrpc.CodeMethodNotFound
}

// CallTool invokes tool with args and returns its raw result.
func (c *Client) CallTool(ctx context.Context, tool string, args map[string]any) (*ToolResponse, error) {
	result, err := c.session.CallTool(ctx, &sdk.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool %q: %w", tool, err)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal result for tool %q: %w", tool, err)
	}
	return &ToolResponse{Raw: raw, IsError: result.IsError}, nil
}

// Close terminates the subprocess and releases the session. It is safe to
// call more than once.
func (c *Client) Close() error {
	return c.session.Close()
}
