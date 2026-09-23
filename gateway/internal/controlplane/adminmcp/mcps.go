package adminmcp

import (
	"context"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolSchemaView is one discovered downstream tool in an mcpView.
type toolSchemaView struct {
	Name         string         `json:"name"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

// mcpView is the MCP-tool representation of a cache.MCPRegistration,
// mirroring the REST admin API's mcpRegistrationDTO.
type mcpView struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Arguments []string          `json:"arguments,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Status    string            `json:"status"`
	Enabled   bool              `json:"enabled"`
	Tools     []toolSchemaView  `json:"tools,omitempty"`
}

func mcpViewFrom(reg cache.MCPRegistration) mcpView {
	v := mcpView{
		Name:      reg.Name,
		Transport: reg.Transport,
		Command:   reg.Connect.Command,
		Arguments: reg.Connect.Arguments,
		Env:       reg.Connect.Env,
		URL:       reg.Connect.URL,
		Status:    reg.Status,
		Enabled:   reg.Enabled,
	}
	for _, ts := range reg.Tools {
		v.Tools = append(v.Tools, toolSchemaView{
			Name:         ts.Name,
			InputSchema:  rawToMap(ts.InputSchema),
			OutputSchema: rawToMap(ts.OutputSchema),
		})
	}
	return v
}

type listMCPsOutput struct {
	MCPs []mcpView `json:"mcps"`
}

type registerMCPInput struct {
	Name      string            `json:"name" jsonschema:"unique MCP identifier"`
	Transport string            `json:"transport,omitempty" jsonschema:"stdio (default), sse, or http"`
	Command   string            `json:"command,omitempty" jsonschema:"binary to execute, for stdio transport"`
	Arguments []string          `json:"arguments,omitempty" jsonschema:"command-line arguments"`
	Env       map[string]string `json:"env,omitempty" jsonschema:"additional environment for the stdio subprocess, beyond the gateway's documented base set"`
	URL       string            `json:"url,omitempty" jsonschema:"endpoint URL, for sse/http transport"`
	Enabled   *bool             `json:"enabled,omitempty" jsonschema:"operator on/off switch; omitted means enabled"`
}

type setMCPEnabledInput struct {
	Name    string `json:"name" jsonschema:"name of an already-registered MCP"`
	Enabled bool   `json:"enabled" jsonschema:"desired state"`
}

// registerMCPTools registers list_mcps, register_mcp, set_mcp_enabled, and
// unregister_mcp on s.
func registerMCPTools(s *sdk.Server, svc *adminservice.MCPService) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_mcps",
		Description: "List every downstream MCP currently registered on the gateway, with its connection status, operator enabled flag, and discovered tools.",
	}, func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listMCPsOutput, error) {
		regs := svc.List()
		out := listMCPsOutput{MCPs: make([]mcpView, 0, len(regs))}
		for _, reg := range regs {
			out.MCPs = append(out.MCPs, mcpViewFrom(reg))
		}
		return nil, out, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "register_mcp",
		Description: "Register a downstream MCP: connect to it, discover its tools, and make it immediately callable through the data plane. Persisted only on success; a connect/discover failure returns a tool error and changes nothing.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in registerMCPInput) (*sdk.CallToolResult, mcpView, error) {
		reg := cache.MCPRegistration{
			Name:      in.Name,
			Transport: in.Transport,
			Connect: cache.ConnectConfig{
				Command:   in.Command,
				Arguments: in.Arguments,
				Env:       in.Env,
				URL:       in.URL,
			},
			Enabled: in.Enabled == nil || *in.Enabled,
		}
		registered, err := svc.Register(ctx, reg)
		if err != nil {
			return nil, mcpView{}, err
		}
		return nil, mcpViewFrom(registered), nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "set_mcp_enabled",
		Description: "Enable or disable an already-registered MCP in place, without reconnecting. A disabled MCP keeps its cached tools and live client; the data plane rejects calls to it with 403 mcp_disabled until it is re-enabled.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in setMCPEnabledInput) (*sdk.CallToolResult, mcpView, error) {
		reg, err := svc.SetEnabled(ctx, in.Name, in.Enabled)
		if err != nil {
			return nil, mcpView{}, err
		}
		return nil, mcpViewFrom(reg), nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "unregister_mcp",
		Description: "Unregister a downstream MCP: close its client, drop it from the registry, and delete its persisted row and schema cache. In-flight calls finish; new calls are rejected. Use set_mcp_enabled to pause an MCP reversibly instead.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, okOutput, error) {
		if err := svc.Unregister(ctx, in.Name); err != nil {
			return nil, okOutput{}, err
		}
		return nil, okOutput{OK: true}, nil
	})
}
