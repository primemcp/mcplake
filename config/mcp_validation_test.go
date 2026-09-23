package config

import (
	"testing"

	"github.com/atsokha/mcplake/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// White-box (package config) so this can reach validateMCP and
// validateSecureHTTPURL directly.

func TestValidateMCP(t *testing.T) {
	tests := []struct {
		name    string
		entry   MCPConfig
		wantErr string
	}{
		{
			name:  "stdio, explicit",
			entry: MCPConfig{Name: "local", Type: "stdio", Command: "some-server"},
		},
		{
			name:  "stdio, type omitted",
			entry: MCPConfig{Name: "local", Command: "some-server"},
		},
		{
			name:  "http with an https url",
			entry: MCPConfig{Name: "remote", Type: "http", URL: "https://mcp.example.com/mcp"},
		},
		{
			name:  "http with a loopback url",
			entry: MCPConfig{Name: "sidecar", Type: "http", URL: "http://localhost:8931/mcp"},
		},
		{
			name:  "sse",
			entry: MCPConfig{Name: "legacy", Type: "sse", URL: "https://mcp.example.com/sse"},
		},
		{
			name:    "no name",
			entry:   MCPConfig{Command: "some-server"},
			wantErr: "name is required",
		},
		{
			name:    "stdio without a command",
			entry:   MCPConfig{Name: "local", Type: "stdio"},
			wantErr: `command is required for type = "stdio"`,
		},
		{
			name:    "http without a url",
			entry:   MCPConfig{Name: "remote", Type: "http"},
			wantErr: `url is required for type = "http"`,
		},
		{
			name:    "plaintext http to a remote host",
			entry:   MCPConfig{Name: "remote", Type: "http", URL: "http://mcp.example.com/mcp"},
			wantErr: "must use https",
		},
		{
			name:    "unknown transport",
			entry:   MCPConfig{Name: "odd", Type: "carrier-pigeon", Command: "x"},
			wantErr: `unsupported type "carrier-pigeon"`,
		},
		// The two cross-field cases exist because getting them wrong is
		// silent: a url on a stdio entry, or a command on an http one, is
		// simply ignored at registration, and the operator is left with an
		// MCP that connects to something other than what they wrote down.
		{
			name:    "a url on a stdio entry",
			entry:   MCPConfig{Name: "confused", Type: "stdio", Command: "x", URL: "https://mcp.example.com/mcp"},
			wantErr: `url is not used by type = "stdio"`,
		},
		{
			name:    "a command on an http entry",
			entry:   MCPConfig{Name: "confused", Type: "http", URL: "https://mcp.example.com/mcp", Command: "x"},
			wantErr: `command is not used by type = "http"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMCP(tt.entry)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// Config.Validate did not look at [[mcps]] at all before ADR-0017, so these
// check the entries are actually reached, and that the error says which one.
func TestValidate_ReportsABadMCPEntryByIndexAndName(t *testing.T) {
	cfg := validConfigWithMCPs(
		MCPConfig{Name: "fine", Command: "some-server"},
		MCPConfig{Name: "broken", Type: "http"},
	)
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcps[1]")
	assert.Contains(t, err.Error(), "(broken)")
	assert.Contains(t, err.Error(), "url is required")
}

func TestValidate_AcceptsAMixOfTransports(t *testing.T) {
	cfg := validConfigWithMCPs(
		MCPConfig{Name: "local", Command: "some-server"},
		MCPConfig{Name: "sidecar", Type: "http", URL: "http://localhost:8931/mcp"},
		MCPConfig{Name: "remote", Type: "sse", URL: "https://mcp.example.com/sse"},
	)
	assert.NoError(t, cfg.Validate())
}

func validConfigWithMCPs(mcps ...MCPConfig) *Config {
	return &Config{
		Server: ServerConfig{DataPlaneAddr: ":8080"},
		OIDC: OIDCConfig{
			JWKSURL:  "https://auth.example.com/.well-known/jwks.json",
			Issuer:   "https://auth.example.com",
			Audience: "mcp-gateway",
		},
		MCPs: mcps,
	}
}

// mcp.ValidateEndpointURL and config.validateSecureHTTPURL implement the
// same rule for different owners: mcp guards a downstream MCP endpoint on
// every write path, config guards oidc.jwks_url and the [admin_auth.login]
// endpoints at startup. Neither can import the other (mcp sits below config
// in the module graph, and a JWKS check has no business living in the mcp
// package), so the duplication is deliberate -- but it must not drift. This
// runs both over one table and fails the moment they disagree, whichever
// one changed.
func TestURLValidatorsAgree(t *testing.T) {
	cases := []string{
		"https://mcp.example.com/mcp",
		"https://mcp.example.com:8443/mcp",
		"http://localhost:8931/mcp",
		"http://LOCALHOST:8931/mcp",
		"http://127.0.0.1:8931/mcp",
		"http://[::1]:8931/mcp",
		"http://mcp.example.com/mcp",
		"http://10.0.0.5:8931/mcp",
		"ftp://mcp.example.com/mcp",
		"/mcp",
		"https://",
		"",
		"http://127.0.0.1.evil.example.com/mcp",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			fromMCP := mcp.ValidateEndpointURL(raw)
			fromConfig := validateSecureHTTPURL(raw)
			assert.Equal(t, fromMCP == nil, fromConfig == nil,
				"the two validators disagree on %q: mcp=%v config=%v", raw, fromMCP, fromConfig)
		})
	}
}
