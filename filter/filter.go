package filter

import (
	"fmt"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/cache"
)

type Filter struct {
	// TODO: Add filtering rules configuration
	schemaCache *cache.SchemaCache
}

type FieldFilterRule struct {
	ClaimKey   string
	ClaimValue string
	MCPName    string
	ToolName   string
	HideFields []string
}

func NewFilter(schemaCache *cache.SchemaCache) *Filter {
	// TODO: Load filtering rules from configuration
	return &Filter{
		schemaCache: schemaCache,
	}
}

func (f *Filter) FilterResponse(claims *auth.Claims, mcpInstance, toolName string, response interface{}) (interface{}, error) {
	// TODO: Get schema for tool
	// TODO: Determine which fields to filter based on claims
	// TODO: Remove fields from response
	// TODO: Return filtered response
	return nil, fmt.Errorf("not implemented")
}

func (f *Filter) AddRule(rule FieldFilterRule) {
	// TODO: Add a new filtering rule
}

func (f *Filter) RemoveRule(rule FieldFilterRule) {
	// TODO: Remove a filtering rule
}
