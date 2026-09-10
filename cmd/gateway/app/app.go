// Package app assembles every component built across Epics #1-#5 into one
// running gateway: config, persistence, the MCP Registry, the Policy Engine,
// the auth Validator, and both HTTP surfaces (data plane and control plane).
// See docs/architecture/overview.md.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/atsokha/mcplake/auth"
	"github.com/atsokha/mcplake/cache"
	"github.com/atsokha/mcplake/config"
	"github.com/atsokha/mcplake/internal"
	"github.com/atsokha/mcplake/internal/controlplane"
	"github.com/atsokha/mcplake/internal/controlplane/adminservice"
	"github.com/atsokha/mcplake/persistence"
	"github.com/atsokha/mcplake/router"
)

// shutdownTimeout bounds how long Run waits for both HTTP surfaces to drain
// in-flight requests once ctx is done.
const shutdownTimeout = 10 * time.Second

// App is a fully wired mcplake gateway: both HTTP surfaces, ready to Run.
type App struct {
	Gateway      *internal.Gateway
	ControlPlane *controlplane.Server
}

// New builds an App from cfg:
//  1. Opens persistence and seeds it from cfg's static mcps/access_policies/
//     filter_policies entries (config.yaml becomes a seed mechanism, the
//     database is the source of truth from then on — ADR-0006).
//  2. Builds the MCP Registry from the persisted registrations and the
//     Policy Engine from the persisted policies.
//  3. Builds the auth Validator (fetches the OIDC provider's JWKS once,
//     synchronously — New fails if that fetch fails).
//  4. Constructs the data-plane Gateway and the control-plane Server, with
//     every admin route wired to the same Registry/repositories/PolicyStore
//     the data plane reads.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	db, err := persistence.Open(cfg.PersistenceConfig())
	if err != nil {
		return nil, fmt.Errorf("app: open persistence: %w", err)
	}

	mcpRepo := persistence.NewMCPRegistrationRepo(db)
	accessRepo := persistence.NewAccessPolicyRepo(db)
	filterRepo := persistence.NewFilterPolicyRepo(db)

	if err := cfg.Seed(ctx, mcpRepo, accessRepo, filterRepo); err != nil {
		return nil, fmt.Errorf("app: seed persistence: %w", err)
	}

	registry := cache.NewRegistry()
	mcpRegistrations, err := mcpRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list mcp registrations: %w", err)
	}
	registry.RegisterAll(ctx, mcpRegistrations)

	accessPolicies, err := accessRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list access policies: %w", err)
	}
	filterPolicies, err := filterRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list filter policies: %w", err)
	}
	policyStore := router.NewPolicyStore(router.NewEngine(accessPolicies, filterPolicies))

	validator, err := auth.NewValidator(ctx, auth.Config{
		JWKSURL:      cfg.OIDC.JWKSURL,
		Issuer:       cfg.OIDC.Issuer,
		Audience:     cfg.OIDC.Audience,
		JWKSCacheTTL: cfg.OIDC.JWKSCacheTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("app: init auth validator: %w", err)
	}

	gateway := internal.NewGateway(internal.Config{
		DataPlaneAddr: cfg.Server.DataPlaneAddr,
		Authenticator: validator,
		Policy:        policyStore,
		Resolver:      registry,
	})

	controlPlaneCfg := controlplane.Config{ControlPlaneAddr: cfg.Server.ControlPlaneAddr}
	if cfg.AdminAuthEnabled() {
		matcher, err := cfg.AdminAuthMatcher()
		if err != nil {
			return nil, fmt.Errorf("app: build admin auth matcher: %w", err)
		}
		controlPlaneCfg.AdminAuth = controlplane.AdminAuth(validator, matcher)
		slog.Info("control-plane admin API authentication enabled",
			"match_rules", len(cfg.AdminAuth.Match))
	} else {
		slog.Warn("control-plane admin API is UNAUTHENTICATED — set admin_auth.match in config, " +
			"or bind server.control_plane_addr to a trusted interface only")
	}
	controlPlane := controlplane.NewServer(controlPlaneCfg)
	reloader := controlplane.NewPolicyReloader(accessRepo, filterRepo, policyStore)
	adminSvc := adminservice.New(registry, mcpRepo, accessRepo, filterRepo, reloader)
	admin := controlPlane.Admin()
	controlplane.RegisterMCPRoutes(admin, adminSvc.MCP)
	controlplane.RegisterAccessPolicyRoutes(admin, adminSvc.Access)
	controlplane.RegisterFilterPolicyRoutes(admin, adminSvc.Filter)
	controlplane.RegisterUIRoutes(controlPlane.Engine(), controlplane.WebUIAssets)

	return &App{Gateway: gateway, ControlPlane: controlPlane}, nil
}

// Run starts both HTTP surfaces and blocks until ctx is done, then
// gracefully shuts both down (draining in-flight requests, up to
// shutdownTimeout) before returning. A start failure on either surface
// causes Run to return promptly without waiting for ctx.
func (a *App) Run(ctx context.Context) error {
	dataPlaneErr := make(chan error, 1)
	controlPlaneErr := make(chan error, 1)

	go func() { dataPlaneErr <- a.Gateway.Start(ctx) }()
	go func() { controlPlaneErr <- a.ControlPlane.Start(ctx) }()

	select {
	case <-a.Gateway.Ready():
	case err := <-dataPlaneErr:
		return fmt.Errorf("app: data plane failed to start: %w", err)
	case <-ctx.Done():
		return a.shutdown(dataPlaneErr, controlPlaneErr)
	}

	select {
	case <-a.ControlPlane.Ready():
	case err := <-controlPlaneErr:
		return fmt.Errorf("app: control plane failed to start: %w", err)
	case <-ctx.Done():
		return a.shutdown(dataPlaneErr, controlPlaneErr)
	}

	<-ctx.Done()
	return a.shutdown(dataPlaneErr, controlPlaneErr)
}

// shutdown gracefully stops both surfaces and waits for their Start calls to
// return, joining any errors encountered.
func (a *App) shutdown(dataPlaneErr, controlPlaneErr <-chan error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var errs []error
	if err := a.Gateway.Stop(shutdownCtx); err != nil {
		errs = append(errs, err)
	}
	if err := a.ControlPlane.Stop(shutdownCtx); err != nil {
		errs = append(errs, err)
	}
	if err := <-dataPlaneErr; err != nil {
		errs = append(errs, err)
	}
	if err := <-controlPlaneErr; err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
