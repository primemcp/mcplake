package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/atsokha/mcplake/router"
)

type Config struct {
	Server         ServerConfig         `toml:"server"`
	OIDC           OIDCConfig           `toml:"oidc"`
	AdminAuth      AdminAuthConfig      `toml:"admin_auth"`
	AdminMCP       AdminMCPConfig       `toml:"admin_mcp"`
	MCP            MCPGlobalConfig      `toml:"mcp"`
	Persistence    PersistenceConfig    `toml:"persistence"`
	MCPs           []MCPConfig          `toml:"mcps"`
	AccessPolicies []AccessPolicyConfig `toml:"access_policies"`
	FilterPolicies []FilterPolicyConfig `toml:"filter_policies"`
}

// MCPGlobalConfig is the `[mcp]` table: settings that apply to every
// registered MCP, distinct from the per-server `[[mcps]]` array.
type MCPGlobalConfig struct {
	// SchemaRefreshInterval, when > 0, re-runs tools/list on every active
	// MCP's existing client on this interval and swaps in the fresh schema
	// (no reconnect). Zero/absent disables periodic refresh — schemas are
	// then fetched only at registration. See ADR-0013.
	SchemaRefreshInterval Duration `toml:"schema_refresh_interval"`
}

// DefaultAdminMCPPath is where the MCP control server mounts when
// admin_mcp.path is omitted. It is under /admin/ so the admin-auth
// middleware covers it. See
// docs/architecture/decisions/0011-mcp-control-server.md.
const DefaultAdminMCPPath = "/admin/mcp"

// AdminMCPConfig configures the in-process MCP control server: the
// control-plane admin operations exposed as MCP tools, mounted on the
// control-plane listener.
type AdminMCPConfig struct {
	// Enabled turns the MCP control server on. It is a pointer only for
	// symmetry with the other on/off switches; unlike them it defaults to
	// OFF — an omitted admin_mcp section mounts nothing.
	Enabled *bool `toml:"enabled"`
	// Path is where the streamable-HTTP handler mounts. Defaults to
	// DefaultAdminMCPPath. Must be under "/admin/" so admin_auth applies.
	Path string `toml:"path"`
}

func (a AdminMCPConfig) enabled() bool {
	return a.Enabled != nil && *a.Enabled
}

func (a AdminMCPConfig) path() string {
	if a.Path == "" {
		return DefaultAdminMCPPath
	}
	return a.Path
}

// AdminAuthConfig configures authentication/authorization for the
// control-plane admin API (`/admin/*`). See
// docs/architecture/decisions/0010-control-plane-admin-authentication.md.
//
// A caller's JWT is verified with the same auth.Validator the data plane
// uses (the oidc: section above); Match then decides whether the verified
// claims belong to an admin.
type AdminAuthConfig struct {
	// Enabled is the operator on/off switch for admin auth. It is a pointer
	// so an omitted key is distinguishable from an explicit `enabled: false`.
	// When nil, admin auth is on iff Match has at least one rule; see
	// AdminAuthConfig.active. An explicit `enabled: true` with an empty Match
	// is a configuration error (it would make every authenticated caller an
	// admin) and is rejected by Config.Validate.
	Enabled *bool `toml:"enabled"`
	// Match is a list of {path, pattern} claim rules, ANDed together, using
	// the same JSONPath+regexp syntax as access_policies (ADR-0002). A
	// verified token is an admin only if every rule matches its claims.
	Match []ClaimRuleConfig `toml:"match"`
}

// active reports whether admin auth should be enforced: an explicit Enabled
// wins; otherwise it is on exactly when at least one Match rule is
// configured. An absent admin_auth section (nil Enabled, no rules) therefore
// leaves the control plane open, matching the documented backward-compatible
// default.
func (a AdminAuthConfig) active() bool {
	if a.Enabled != nil {
		return *a.Enabled
	}
	return len(a.Match) > 0
}

// PersistenceConfig is the config shape of a persistence.Config. See
// ADR-0006: Driver is "sqlite" (default) or "postgres"; DSN is the SQLite
// file path or the PostgreSQL connection string, depending on Driver.
type PersistenceConfig struct {
	Driver string `toml:"driver"`
	DSN    string `toml:"dsn"`
}

type ServerConfig struct {
	// DataPlaneAddr is the listen address (host:port) for the fasthttp
	// tool-call proxy. See ADR-0001.
	DataPlaneAddr string `toml:"data_plane_addr"`
	// ControlPlaneAddr is the listen address (host:port) for the Gin admin
	// API. See ADR-0005.
	ControlPlaneAddr string `toml:"control_plane_addr"`
	// TODO: Add TLS configuration
}

type OIDCConfig struct {
	// JWKSURL is the OIDC provider's JWKS endpoint. See auth.Config.JWKSURL;
	// OIDC discovery from a provider/issuer URL is not yet implemented.
	JWKSURL string `toml:"jwks_url"`
	// Issuer is the required `iss` claim value. See auth.Config.Issuer.
	Issuer string `toml:"issuer"`
	// Audience is the required `aud` claim value. See auth.Config.Audience.
	Audience string `toml:"audience"`
	// JWKSCacheTTL is how long fetched keys are cached before a background
	// refresh, as a Go duration string ("1h"). Absent/empty leaves it 0, so
	// auth.NewValidator applies its own default. See auth.Config.JWKSCacheTTL.
	JWKSCacheTTL Duration `toml:"jwks_cache_ttl"`
}

type MCPConfig struct {
	Name      string   `toml:"name"`
	Type      string   `toml:"type"` // "stdio", "sse", etc.
	Command   string   `toml:"command"`
	Arguments []string `toml:"arguments"`
	// Enabled is the operator on/off switch for this MCP. It is a pointer so
	// an omitted key is distinguishable from an explicit `enabled = false`;
	// omitted defaults to enabled. See enabledOrDefault.
	Enabled *bool `toml:"enabled"`
}

// ClaimRuleConfig is the config shape of a router.ClaimRule. See ADR-0002.
type ClaimRuleConfig struct {
	Path    string `toml:"path"`
	Pattern string `toml:"pattern"`
}

// GrantConfig is the config shape of a router.Grant.
type GrantConfig struct {
	MCP   string   `toml:"mcp"`
	Tools []string `toml:"tools"`
}

// AccessPolicyConfig is the config shape of a router.AccessPolicy. See
// ADR-0004 and docs/architecture/data.md#access-policy.
type AccessPolicyConfig struct {
	Name   string            `toml:"name"`
	Match  []ClaimRuleConfig `toml:"match"`
	Grants []GrantConfig     `toml:"grants"`
	// Enabled is the operator on/off switch for this policy. A pointer so an
	// omitted key is distinguishable from an explicit `enabled = false`;
	// omitted defaults to enabled. See enabledOrDefault.
	Enabled *bool `toml:"enabled"`
}

// FilterPolicyConfig is the config shape of a router.FilterPolicy. See
// ADR-0004 and docs/architecture/data.md#filter-policy.
type FilterPolicyConfig struct {
	Name       string            `toml:"name"`
	Match      []ClaimRuleConfig `toml:"match"`
	MCP        string            `toml:"mcp"`
	Tool       string            `toml:"tool"`
	DropFields []string          `toml:"drop_fields"`
	// Enabled is the operator on/off switch for this policy. A pointer so an
	// omitted key is distinguishable from an explicit `enabled = false`;
	// omitted defaults to enabled. See enabledOrDefault.
	Enabled *bool `toml:"enabled"`
}

// Load reads and parses the TOML configuration file at path (see ADR-0012),
// then runs Validate on the result so a malformed policy rule (or any other
// structural problem Validate checks) is reported at load time rather than
// discovered later during request handling.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate checks that Config is structurally sound before the gateway
// starts serving. Its primary purpose (per ADR-0002/ADR-0004) is compiling
// every AccessPolicy/FilterPolicy claim rule up front: a malformed JSONPath
// expression or regexp is a configuration error to catch at startup, not a
// per-request failure discovered later.
func (c *Config) Validate() error {
	if c.Server.DataPlaneAddr == "" {
		return fmt.Errorf("config: server.data_plane_addr is required")
	}
	if c.OIDC.JWKSURL == "" {
		return fmt.Errorf("config: oidc.jwks_url is required")
	}
	if c.OIDC.Issuer == "" {
		return fmt.Errorf("config: oidc.issuer is required")
	}
	if c.OIDC.Audience == "" {
		return fmt.Errorf("config: oidc.audience is required")
	}

	if c.AdminAuth.Enabled != nil && *c.AdminAuth.Enabled && len(c.AdminAuth.Match) == 0 {
		return fmt.Errorf("config: admin_auth.match must contain at least one rule when admin_auth.enabled is true")
	}
	if err := validateClaimRules(c.AdminAuth.Match); err != nil {
		return fmt.Errorf("config: admin_auth: %w", err)
	}

	if c.AdminMCP.Path != "" && !strings.HasPrefix(c.AdminMCP.Path, "/admin/") {
		return fmt.Errorf("config: admin_mcp.path must be under /admin/ (got %q)", c.AdminMCP.Path)
	}

	if c.MCP.SchemaRefreshInterval.Duration < 0 {
		return fmt.Errorf("config: mcp.schema_refresh_interval must not be negative (got %s)", c.MCP.SchemaRefreshInterval.Duration)
	}

	for i, policy := range c.AccessPolicies {
		if policy.Name == "" {
			return fmt.Errorf("config: access_policies[%d]: name is required", i)
		}
		if err := validateClaimRules(policy.Match); err != nil {
			return fmt.Errorf("config: access_policies[%d] (%s): %w", i, policy.Name, err)
		}
		for j, grant := range policy.Grants {
			if grant.MCP == "" {
				return fmt.Errorf("config: access_policies[%d] (%s): grants[%d]: mcp is required", i, policy.Name, j)
			}
		}
	}

	for i, policy := range c.FilterPolicies {
		if policy.Name == "" {
			return fmt.Errorf("config: filter_policies[%d]: name is required", i)
		}
		if policy.MCP == "" {
			return fmt.Errorf("config: filter_policies[%d] (%s): mcp is required", i, policy.Name)
		}
		if policy.Tool == "" {
			return fmt.Errorf("config: filter_policies[%d] (%s): tool is required", i, policy.Name)
		}
		if err := validateClaimRules(policy.Match); err != nil {
			return fmt.Errorf("config: filter_policies[%d] (%s): %w", i, policy.Name, err)
		}
	}

	return nil
}

// enabledOrDefault resolves a config `enabled` pointer to a concrete bool:
// an omitted key (nil) means enabled, matching the documented default in
// docs/CONFIG.md. An explicit `enabled: false` disables the entry.
func enabledOrDefault(enabled *bool) bool {
	return enabled == nil || *enabled
}

// validateClaimRules compiles every rule via router.NewClaimRule, returning
// the first error encountered, identifying which rule failed.
func validateClaimRules(rules []ClaimRuleConfig) error {
	for i, rule := range rules {
		if _, err := router.NewClaimRule(rule.Path, rule.Pattern); err != nil {
			return fmt.Errorf("match[%d]: %w", i, err)
		}
	}
	return nil
}
