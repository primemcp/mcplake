package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test file is package cache (white-box), not cache_test, because it
// needs the unexported set method to seed a Registry directly — Register
// (the real write path) doesn't exist until ticket #17.

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "postgres-ro",
		Status: StatusActive,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
	})

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusActive, got.Status)

	_, ok = r.Get("does-not-exist")
	assert.False(t, ok)
}

func TestRegistry_HasTool(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "postgres-ro",
		Status: StatusActive,
		Tools: map[string]ToolSchema{
			"get_user": {Name: "get_user"},
		},
	})

	assert.True(t, r.HasTool("postgres-ro", "get_user"))
	assert.False(t, r.HasTool("postgres-ro", "delete_user"), "unknown tool on a known MCP")
	assert.False(t, r.HasTool("unknown-mcp", "get_user"), "unknown MCP")
}

func TestRegistry_HasTool_MCPWithNoTools(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{Name: "postgres-ro", Status: StatusConnecting})

	assert.False(t, r.HasTool("postgres-ro", "get_user"))
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{Name: "postgres-ro", Status: StatusActive})
	r.set(MCPRegistration{Name: "postgres-rw", Status: StatusActive})

	list := r.List()

	assert.Len(t, list, 2)
	names := []string{list[0].Name, list[1].Name}
	assert.ElementsMatch(t, []string{"postgres-ro", "postgres-rw"}, names)
}

func TestRegistry_List_DefensiveCopy(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{Name: "postgres-ro", Status: StatusActive})

	list := r.List()
	list[0].Status = "mutated"

	got, ok := r.Get("postgres-ro")
	require.True(t, ok)
	assert.Equal(t, StatusActive, got.Status, "mutating the returned slice must not affect Registry state")
}

func TestRegistry_EmptyRegistryDoesNotPanic(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("anything")
	assert.False(t, ok)
	assert.False(t, r.HasTool("anything", "anything"))
	assert.Empty(t, r.List())
}

func TestRegistry_ConcurrentReadsAreRaceFree(t *testing.T) {
	r := NewRegistry()
	r.set(MCPRegistration{
		Name:   "postgres-ro",
		Status: StatusActive,
		Tools:  map[string]ToolSchema{"get_user": {Name: "get_user"}},
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			r.Get("postgres-ro")
		}()
		go func() {
			defer wg.Done()
			r.HasTool("postgres-ro", "get_user")
		}()
		go func() {
			defer wg.Done()
			r.List()
		}()
	}
	wg.Wait()
}
