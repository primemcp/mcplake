package config

import (
	"fmt"
	"net"
	"net/url"
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
	// Login describes how the embedded admin web UI obtains a token from
	// the OIDC provider. Optional: an absent table just means the UI has
	// no login flow configured (it then says so rather than offering a
	// broken button). See ADR-0014.
	Login AdminLoginConfig `toml:"login"`
}

// DefaultAdminLoginScopes is requested when admin_auth.login.scopes is
// omitted: `openid` is required for an OIDC authorization request at all,
// and profile/email are what the UI shows as the signed-in identity.
var DefaultAdminLoginScopes = []string{"openid", "profile", "email"}

// AdminLoginConfig is the `[admin_auth.login]` table: the OIDC coordinates
// the admin web UI needs to run an Authorization Code + PKCE flow against
// the same provider `[oidc]` already verifies tokens from. See ADR-0014.
//
// There is deliberately no client secret: the UI is a public client whose
// bundle is readable by anyone who loads the page, which is exactly the
// case PKCE exists for. Endpoints are explicit rather than discovered from
// the issuer, mirroring OIDCConfig.JWKSURL and keeping startup free of any
// outbound request (the project targets zero-egress deployments).
type AdminLoginConfig struct {
	// ClientID is the public client registered at the provider for this
	// UI. Its allowed redirect URIs must include the control-plane URL
	// operators browse to.
	ClientID string `toml:"client_id"`
	// AuthorizationEndpoint is where the browser is redirected to sign in.
	AuthorizationEndpoint string `toml:"authorization_endpoint"`
	// TokenEndpoint is where the UI exchanges the authorization code for a
	// token, using its PKCE code_verifier.
	TokenEndpoint string `toml:"token_endpoint"`
	// Scopes requested in the authorization request. Empty means
	// DefaultAdminLoginScopes. The resulting token must satisfy
	// oidc.audience, which for many providers is a matter of scope or a
	// provider-side audience mapping.
	Scopes []string `toml:"scopes"`
}

// ScopesOrDefault resolves Scopes to what should actually be requested.
func (l AdminLoginConfig) ScopesOrDefault() []string {
	if len(l.Scopes) == 0 {
		return DefaultAdminLoginScopes
	}
	return l.Scopes
}

// configured reports whether the table carries a usable login flow. A
// partially filled table is rejected by Config.Validate, so by the time
// anything calls this it is either complete or empty.
func (l AdminLoginConfig) configured() bool {
	return l.ClientID != "" && l.AuthorizationEndpoint != "" && l.TokenEndpoint != ""
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
	Name string `toml:"name"`
	// Type is the transport: "stdio" (the default), "http" or "sse".
	// See ADR-0017.
	Type string `toml:"type"`
	// Command and Arguments are the subprocess to spawn, for stdio.
	Command   string   `toml:"command"`
	Arguments []string `toml:"arguments"`
	// URL is the server's endpoint, for http and sse. It must be https, or
	// http on a loopback host.
	URL string `toml:"url"`
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
	if err := validateSecureHTTPURL(c.OIDC.JWKSURL); err != nil {
		return fmt.Errorf("config: oidc.jwks_url: %w", err)
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
	if err := validateAdminLogin(c.AdminAuth.Login); err != nil {
		return err
	}

	if c.AdminMCP.Path != "" && !strings.HasPrefix(c.AdminMCP.Path, "/admin/") {
		return fmt.Errorf("config: admin_mcp.path must be under /admin/ (got %q)", c.AdminMCP.Path)
	}

	if c.MCP.SchemaRefreshInterval.Duration < 0 {
		return fmt.Errorf("config: mcp.schema_refresh_interval must not be negative (got %s)", c.MCP.SchemaRefreshInterval.Duration)
	}

	for i, m := range c.MCPs {
		if err := validateMCP(m); err != nil {
			return fmt.Errorf("config: mcps[%d]%s: %w", i, namedSuffix(m.Name), err)
		}
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

// validateAdminLogin checks the [admin_auth.login] table is either wholly
// absent or complete and usable. A half-filled table is a mistake, not an
// "off" signal: the UI would build an authorization request with an empty
// client_id or redirect to an empty URL. The endpoints are validated as
// absolute http(s) URLs here, at startup, rather than failing in the
// operator's browser at their first login. See ADR-0014.
func validateAdminLogin(l AdminLoginConfig) error {
	set := map[string]string{
		"client_id":              l.ClientID,
		"authorization_endpoint": l.AuthorizationEndpoint,
		"token_endpoint":         l.TokenEndpoint,
	}
	filled := 0
	for _, v := range set {
		if v != "" {
			filled++
		}
	}
	if filled == 0 && len(l.Scopes) == 0 {
		return nil
	}
	for _, key := range []string{"client_id", "authorization_endpoint", "token_endpoint"} {
		if set[key] == "" {
			return fmt.Errorf("config: admin_auth.login: %s is required", key)
		}
	}
	for _, ep := range []struct{ key, value string }{
		{"authorization_endpoint", l.AuthorizationEndpoint},
		{"token_endpoint", l.TokenEndpoint},
	} {
		if err := validateSecureHTTPURL(ep.value); err != nil {
			return fmt.Errorf("config: admin_auth.login.%s: %w", ep.key, err)
		}
	}
	return nil
}

// validateSecureHTTPURL accepts an absolute https URL, or an absolute http
// URL whose host is loopback.
//
// The URLs this guards carry security-critical material: the JWKS is the
// root of trust for every token the gateway accepts, and the OIDC token
// endpoint receives the PKCE code and code_verifier from the operator's
// browser. Over plaintext, anyone on the path can substitute a signing key
// or read the exchange. The gateway itself terminates no TLS (see
// ServerConfig), so this is a guardrail against operator error, not a
// claim that the deployment is encrypted end to end.
//
// Loopback keeps plaintext because the demo stack, the httptest fixtures
// and local development providers all serve over an interface no attacker
// sits on, and forcing certificates there would buy nothing.
func validateSecureHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must be an absolute http(s) URL (got %q)", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("must include a host (got %q)", raw)
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("must use https (got %q); plaintext http is accepted only for a loopback host", raw)
	}
	return nil
}

// isLoopbackHost reports whether host names the local machine. url.URL's
// Hostname strips the port and the brackets around an IPv6 literal, so
// "[::1]:9999" arrives here as "::1".
//
// The "localhost" comparison is case-insensitive: hostnames are
// case-insensitive per RFC 4343 and url.URL.Hostname performs no
// normalization, so a literal "LOCALHOST" (a plausible copy-paste or
// Windows-style entry) resolves to the loopback interface exactly like
// "localhost" does and must not be rejected as if it were a remote host.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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
