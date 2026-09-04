package config_test

import (
	"testing"

	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_MCPRegistrations_MapsFields(t *testing.T) {
	cfg := config.Config{
		MCPs: []config.MCPConfig{
			{
				Name:      "postgres-ro",
				Type:      "stdio",
				Command:   "mcp-server-postgres",
				Arguments: []string{"--read-only"},
			},
		},
	}

	regs := cfg.MCPRegistrations()

	require.Len(t, regs, 1)
	assert.Equal(t, "postgres-ro", regs[0].Name)
	assert.Equal(t, "stdio", regs[0].Transport)
	assert.Equal(t, cache.ConnectConfig{
		Command:   "mcp-server-postgres",
		Arguments: []string{"--read-only"},
	}, regs[0].Connect)
}

func TestConfig_MCPRegistrations_DefaultsEmptyTypeToStdio(t *testing.T) {
	cfg := config.Config{
		MCPs: []config.MCPConfig{
			{Name: "filesystem", Command: "mcp-server-filesystem"},
		},
	}

	regs := cfg.MCPRegistrations()

	require.Equal(t, "stdio", regs[0].Transport)
}

func TestConfig_MCPRegistrations_PreservesExplicitNonStdioType(t *testing.T) {
	cfg := config.Config{
		MCPs: []config.MCPConfig{
			{Name: "remote-mcp", Type: "sse", Command: "unused"},
		},
	}

	regs := cfg.MCPRegistrations()

	assert.Equal(t, "sse", regs[0].Transport)
}

func TestConfig_MCPRegistrations_EmptyMCPsReturnsEmptySlice(t *testing.T) {
	cfg := config.Config{}

	regs := cfg.MCPRegistrations()

	assert.Empty(t, regs)
}

func TestConfig_MCPRegistrations_PreservesOrderAndCount(t *testing.T) {
	cfg := config.Config{
		MCPs: []config.MCPConfig{
			{Name: "a", Command: "cmd-a"},
			{Name: "b", Command: "cmd-b"},
			{Name: "c", Command: "cmd-c"},
		},
	}

	regs := cfg.MCPRegistrations()

	require.Len(t, regs, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{regs[0].Name, regs[1].Name, regs[2].Name})
}
