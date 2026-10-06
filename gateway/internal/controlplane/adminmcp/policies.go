package adminmcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/primemcp/mcplake/internal/controlplane/adminservice"
	"github.com/primemcp/mcplake/router"
)

// claimRuleView is one {path, pattern} rule, shared by the access- and
// filter-policy tool shapes.
type claimRuleView struct {
	Path    string `json:"path" jsonschema:"JSONPath into the decoded JWT claim set"`
	Pattern string `json:"pattern" jsonschema:"RE2 regexp tested against the value(s) at path"`
}

type grantView struct {
	MCP   string   `json:"mcp" jsonschema:"registered MCP name, or * for all"`
	Tools []string `json:"tools" jsonschema:"tool names, or [\"*\"] for all tools on the MCP"`
}

type accessPolicyView struct {
	Name    string          `json:"name"`
	Match   []claimRuleView `json:"match"`
	Grants  []grantView     `json:"grants"`
	Enabled bool            `json:"enabled"`
}

func accessPolicyViewFrom(p router.AccessPolicy) accessPolicyView {
	v := accessPolicyView{Name: p.Name, Enabled: p.Enabled}
	for _, r := range p.Match.Rules {
		v.Match = append(v.Match, claimRuleView{Path: r.Path, Pattern: r.Pattern})
	}
	for _, g := range p.Grants {
		v.Grants = append(v.Grants, grantView{MCP: g.MCP, Tools: g.Tools})
	}
	return v
}

type accessPolicyInput struct {
	Name    string          `json:"name" jsonschema:"unique policy identifier"`
	Match   []claimRuleView `json:"match,omitempty" jsonschema:"claim rules, ANDed together"`
	Grants  []grantView     `json:"grants,omitempty" jsonschema:"(mcp, tools) pairs this policy grants"`
	Enabled *bool           `json:"enabled,omitempty" jsonschema:"operator on/off switch; omitted means enabled"`
}

func (in accessPolicyInput) toPolicy() router.AccessPolicy {
	p := router.AccessPolicy{Name: in.Name, Enabled: in.Enabled == nil || *in.Enabled}
	rules := make([]router.ClaimRule, len(in.Match))
	for i, r := range in.Match {
		rules[i] = router.ClaimRule{Path: r.Path, Pattern: r.Pattern}
	}
	p.Match = router.ClaimMatcher{Rules: rules}
	p.Grants = make([]router.Grant, len(in.Grants))
	for i, g := range in.Grants {
		p.Grants[i] = router.Grant{MCP: g.MCP, Tools: g.Tools}
	}
	return p
}

type listAccessPoliciesOutput struct {
	Policies []accessPolicyView `json:"policies"`
}

func registerAccessPolicyTools(s *sdk.Server, svc *adminservice.AccessPolicyService) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_access_policies",
		Description: "List every stored access policy. Access policies are additive: a call is authorized if any enabled policy's match rules match the caller's claims and one of its grants covers the (mcp, tool).",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listAccessPoliciesOutput, error) {
		policies, err := svc.List(ctx)
		if err != nil {
			return nil, listAccessPoliciesOutput{}, err
		}
		out := listAccessPoliciesOutput{Policies: make([]accessPolicyView, 0, len(policies))}
		for _, p := range policies {
			out.Policies = append(out.Policies, accessPolicyViewFrom(p))
		}
		return nil, out, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_access_policy",
		Description: "Fetch one stored access policy by name.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, accessPolicyView, error) {
		p, err := svc.Get(ctx, in.Name)
		if err != nil {
			return nil, accessPolicyView{}, err
		}
		return nil, accessPolicyViewFrom(p), nil
	})

	upsert := func(ctx context.Context, in accessPolicyInput) (*sdk.CallToolResult, accessPolicyView, error) {
		p := in.toPolicy()
		if err := svc.Upsert(ctx, p); err != nil {
			return nil, accessPolicyView{}, err
		}
		return nil, accessPolicyViewFrom(p), nil
	}
	sdk.AddTool(s, &sdk.Tool{
		Name:        "create_access_policy",
		Description: "Create an access policy (adds it, or overwrites an existing policy of the same name). A match rule whose path or pattern fails to compile is rejected and nothing is stored.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in accessPolicyInput) (*sdk.CallToolResult, accessPolicyView, error) {
		return upsert(ctx, in)
	})
	sdk.AddTool(s, &sdk.Tool{
		Name:        "replace_access_policy",
		Description: "Replace the named access policy in full (creating it if absent). Omitting enabled re-enables the policy.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in accessPolicyInput) (*sdk.CallToolResult, accessPolicyView, error) {
		return upsert(ctx, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "delete_access_policy",
		Description: "Delete the named access policy. The change is visible to the data plane immediately.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, okOutput, error) {
		if err := svc.Delete(ctx, in.Name); err != nil {
			return nil, okOutput{}, err
		}
		return nil, okOutput{OK: true}, nil
	})
}

type filterPolicyView struct {
	Name       string          `json:"name"`
	Match      []claimRuleView `json:"match"`
	MCP        string          `json:"mcp"`
	Tool       string          `json:"tool"`
	DropFields []string        `json:"drop_fields"`
	Enabled    bool            `json:"enabled"`
}

func filterPolicyViewFrom(p router.FilterPolicy) filterPolicyView {
	v := filterPolicyView{
		Name:       p.Name,
		MCP:        p.MCP,
		Tool:       p.Tool,
		DropFields: p.DropFields,
		Enabled:    p.Enabled,
	}
	for _, r := range p.Match.Rules {
		v.Match = append(v.Match, claimRuleView{Path: r.Path, Pattern: r.Pattern})
	}
	return v
}

type filterPolicyInput struct {
	Name       string          `json:"name" jsonschema:"unique policy identifier"`
	Match      []claimRuleView `json:"match,omitempty" jsonschema:"claim rules, ANDed together"`
	MCP        string          `json:"mcp" jsonschema:"exact MCP name this filter targets (no wildcards)"`
	Tool       string          `json:"tool" jsonschema:"exact tool name this filter targets (no wildcards)"`
	DropFields []string        `json:"drop_fields,omitempty" jsonschema:"JSONPath expressions to strip from the tool response"`
	Enabled    *bool           `json:"enabled,omitempty" jsonschema:"operator on/off switch; omitted means enabled"`
}

func (in filterPolicyInput) toPolicy() router.FilterPolicy {
	p := router.FilterPolicy{
		Name:       in.Name,
		MCP:        in.MCP,
		Tool:       in.Tool,
		DropFields: in.DropFields,
		Enabled:    in.Enabled == nil || *in.Enabled,
	}
	rules := make([]router.ClaimRule, len(in.Match))
	for i, r := range in.Match {
		rules[i] = router.ClaimRule{Path: r.Path, Pattern: r.Pattern}
	}
	p.Match = router.ClaimMatcher{Rules: rules}
	return p
}

type listFilterPoliciesOutput struct {
	Policies []filterPolicyView `json:"policies"`
}

func registerFilterPolicyTools(s *sdk.Server, svc *adminservice.FilterPolicyService) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "list_filter_policies",
		Description: "List every stored filter policy. A filter policy strips response fields for one exact (mcp, tool) when the caller's claims match; the union of drop_fields across all matching enabled policies is removed.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, listFilterPoliciesOutput, error) {
		policies, err := svc.List(ctx)
		if err != nil {
			return nil, listFilterPoliciesOutput{}, err
		}
		out := listFilterPoliciesOutput{Policies: make([]filterPolicyView, 0, len(policies))}
		for _, p := range policies {
			out.Policies = append(out.Policies, filterPolicyViewFrom(p))
		}
		return nil, out, nil
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "get_filter_policy",
		Description: "Fetch one stored filter policy by name.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, filterPolicyView, error) {
		p, err := svc.Get(ctx, in.Name)
		if err != nil {
			return nil, filterPolicyView{}, err
		}
		return nil, filterPolicyViewFrom(p), nil
	})

	upsert := func(ctx context.Context, in filterPolicyInput) (*sdk.CallToolResult, filterPolicyView, error) {
		p := in.toPolicy()
		if err := svc.Upsert(ctx, p); err != nil {
			return nil, filterPolicyView{}, err
		}
		return nil, filterPolicyViewFrom(p), nil
	}
	sdk.AddTool(s, &sdk.Tool{
		Name:        "create_filter_policy",
		Description: "Create a filter policy (adds it, or overwrites an existing policy of the same name). mcp and tool are exact, no wildcards. A match rule that fails to compile is rejected and nothing is stored.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in filterPolicyInput) (*sdk.CallToolResult, filterPolicyView, error) {
		return upsert(ctx, in)
	})
	sdk.AddTool(s, &sdk.Tool{
		Name:        "replace_filter_policy",
		Description: "Replace the named filter policy in full (creating it if absent). Omitting enabled re-enables the policy.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in filterPolicyInput) (*sdk.CallToolResult, filterPolicyView, error) {
		return upsert(ctx, in)
	})

	sdk.AddTool(s, &sdk.Tool{
		Name:        "delete_filter_policy",
		Description: "Delete the named filter policy. Other enabled filter policies for the same (mcp, tool) still apply.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in nameInput) (*sdk.CallToolResult, okOutput, error) {
		if err := svc.Delete(ctx, in.Name); err != nil {
			return nil, okOutput{}, err
		}
		return nil, okOutput{OK: true}, nil
	})
}

type healthOutput struct {
	Status string `json:"status" jsonschema:"ok when the control plane is serving"`
}

func registerHealthTool(s *sdk.Server) {
	sdk.AddTool(s, &sdk.Tool{
		Name:        "gateway_health",
		Description: "Report control-plane liveness. Returns status \"ok\" when the admin surface is serving; the MCP connection itself succeeding already implies this.",
	}, func(_ context.Context, _ *sdk.CallToolRequest, _ struct{}) (*sdk.CallToolResult, healthOutput, error) {
		return nil, healthOutput{Status: "ok"}, nil
	})
}
