package internal

import (
	"context"
	"fmt"
	"net/http"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/atsokha/mcplake/filter"
	"github.com/atsokha/mcplake/router"
)

type Gateway struct {
	config        *config.Config
	authValidator *auth.Validator
	schemaCache   *cache.SchemaCache
	router        *router.Router
	filter        *filter.Filter
	server        *http.Server
}

func NewGateway(
	cfg *config.Config,
	authValidator *auth.Validator,
	schemaCache *cache.SchemaCache,
	r *router.Router,
	f *filter.Filter,
) (*Gateway, error) {
	return &Gateway{
		config:        cfg,
		authValidator: authValidator,
		schemaCache:   schemaCache,
		router:        r,
		filter:        f,
	}, nil
}

func (g *Gateway) Start(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (g *Gateway) Stop(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (g *Gateway) HandleToolCall(w http.ResponseWriter, r *http.Request) {
	// TODO: Extract JWT from request
	// TODO: Validate JWT
	// TODO: Extract claims
	// TODO: Route request
	// TODO: Call MCP tool
	// TODO: Filter response
	// TODO: Write response
}
