package router

import (
	"fmt"

	"github.com/primemcp/mcplake/auth"
)

type Router struct {
	// TODO: Add routing rules configuration
	// TODO: Add MCP instance registry
}

type RoutingRule struct {
	ClaimKey    string
	ClaimValue  string
	MCPInstance string
	Priority    int
}

func NewRouter() *Router {
	// TODO: Initialize router with configuration
	return &Router{}
}

func (r *Router) Route(claims *auth.Claims, toolName string) (string, error) {
	// TODO: Implement routing logic based on claims
	// TODO: Match claims to routing rules
	// TODO: Return MCP instance name
	return "", fmt.Errorf("not implemented")
}

func (r *Router) RegisterMCPInstance(name string, config interface{}) error {
	// TODO: Register a new MCP instance
	return fmt.Errorf("not implemented")
}

func (r *Router) UnregisterMCPInstance(name string) error {
	// TODO: Unregister an MCP instance
	return fmt.Errorf("not implemented")
}
