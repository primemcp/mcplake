package config_test

import (
	"testing"

	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The compose demo's config lives in this repo and is loaded by the same
// Load/Validate path as any other, but nothing in CI was exercising it --
// so a tightening of the rules (ADR-0017 added the first validation
// [[mcps]] ever had) could leave it failing, and the only signal would be an
// operator's `compose up` crashing. config.example.toml already gets this
// treatment a few files over; this extends it to the file people actually
// run.
func TestDemoConfig_LoadsAndValidates(t *testing.T) {
	cfg, err := config.Load("../deploy/demo/config.toml")
	require.NoError(t, err, "deploy/demo/config.toml must load and validate")

	byName := make(map[string]config.MCPConfig, len(cfg.MCPs))
	for _, m := range cfg.MCPs {
		byName[m.Name] = m
	}

	// The demo's whole point since ADR-0017 is that it runs one MCP each
	// way. If either of these changes shape, the demo walkthrough (docs/getting-started/demo.rst) built
	// on them is wrong too.
	stdio, ok := byName["employee-directory"]
	require.True(t, ok, "the stdio MCP should still be there")
	assert.Equal(t, "stdio", stdio.Type)
	assert.NotEmpty(t, stdio.Command)

	remote, ok := byName["demo-postgres"]
	require.True(t, ok, "the out-of-container MCP should still be there")
	assert.Contains(t, []string{"http", "sse"}, remote.Type)
	assert.NotEmpty(t, remote.URL)

	// Not a style preference: mcp.ValidateEndpointURL rejects plaintext to
	// anything but a loopback host, and the demo only gets away with http
	// because compose puts that container in the gateway's own network
	// namespace. Swap in a service hostname and the gateway refuses to
	// start -- fail here instead, where the reason is written down.
	assert.Contains(t, remote.URL, "localhost",
		"the demo's remote MCP URL must stay loopback; see compose.yaml's network_mode")

	// Inspector connects to the control server, so the demo is broken if
	// this is ever turned back off.
	assert.True(t, cfg.AdminMCPEnabled(), "the demo enables the MCP control server for Inspector")
}
