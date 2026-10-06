// Package app assembles every component built across Epics #1-#5 into one
// running gateway: config, persistence, the MCP Registry, the Policy Engine,
// the auth Validator, and both HTTP surfaces (data plane and control plane).
// See docs/architecture/overview.rst.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/primemcp/mcplake/auth"
	"github.com/primemcp/mcplake/cache"
	"github.com/primemcp/mcplake/config"
	"github.com/primemcp/mcplake/internal"
	"github.com/primemcp/mcplake/internal/controlplane"
	"github.com/primemcp/mcplake/internal/controlplane/adminservice"
	"github.com/primemcp/mcplake/persistence"
	"github.com/primemcp/mcplake/router"
)

// shutdownTimeout bounds how long Run waits for both HTTP surfaces to drain
// in-flight requests once ctx is done.
const shutdownTimeout = 10 * time.Second

// App is a fully wired mcplake gateway: both HTTP surfaces, ready to Run.
type App struct {
	Gateway      *internal.Gateway
	ControlPlane *controlplane.Server
	// refresher periodically re-discovers MCP tool schemas. Nil when
	// mcp.schema_refresh_interval is unset/zero (ADR-0013).
	refresher *cache.SchemaRefresher
	// healthChecker periodically pings every registered MCP and reconnects
	// the ones that have stopped answering. Nil only when an operator set
	// mcp.health_check_interval to 0 — unlike the refresher it is on by
	// default (ADR-0019).
	healthChecker *cache.HealthChecker
}

// New builds an App from cfg:
//  1. Opens persistence and seeds it from cfg's static mcps/access_policies/
//     filter_policies entries (the config file becomes a seed mechanism, the
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

	seedMarkerRepo := persistence.NewSeedMarkerRepo(db)
	mcpRepo := persistence.NewMCPRegistrationRepo(db)
	accessRepo := persistence.NewAccessPolicyRepo(db)
	filterRepo := persistence.NewFilterPolicyRepo(db)

	if err := cfg.Seed(ctx, seedMarkerRepo, mcpRepo, accessRepo, filterRepo); err != nil {
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
		JWKSCacheTTL: cfg.OIDC.JWKSCacheTTL.Duration,
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
		controlPlaneCfg.AuthInfo = adminAuthInfo(cfg)
		slog.Info("control-plane admin API authentication enabled",
			"match_rules", len(cfg.AdminAuth.Match))
		if !controlPlaneCfg.AuthInfo.LoginConfigured() {
			slog.Warn("admin web UI has no login flow — admin auth is on but [admin_auth.login] is unset, " +
				"so the UI cannot obtain a token and every call from it will fail with 401 (see ADR-0014)")
		}
	} else {
		slog.Warn("control-plane admin API is UNAUTHENTICATED — set admin_auth.match in config, " +
			"or bind server.control_plane_addr to a trusted interface only")
	}
	controlPlane := controlplane.NewServer(controlPlaneCfg)
	reloader := controlplane.NewPolicyReloader(accessRepo, filterRepo, policyStore)
	adminSvc := adminservice.New(ctx, registry, mcpRepo, accessRepo, filterRepo, reloader)
	admin := controlPlane.Admin()
	controlplane.RegisterMCPRoutes(admin, adminSvc.MCP)
	controlplane.RegisterAccessPolicyRoutes(admin, adminSvc.Access)
	controlplane.RegisterFilterPolicyRoutes(admin, adminSvc.Filter)
	if cfg.AdminMCPEnabled() {
		controlplane.RegisterMCPControlServer(admin, adminSvc, cfg.AdminMCPPath())
		slog.Info("MCP control server mounted on the control plane", "path", cfg.AdminMCPPath())
		if !cfg.AdminAuthEnabled() {
			slog.Warn("MCP control server is UNAUTHENTICATED — it inherits admin_auth, which is not configured")
		}
	}
	controlplane.RegisterUIRoutes(controlPlane.Engine(), controlplane.WebUIAssets)

	var refresher *cache.SchemaRefresher
	if interval := cfg.SchemaRefreshInterval(); interval > 0 {
		refresher = cache.NewSchemaRefresher(registry, interval)
		slog.Info("periodic MCP schema refresh enabled", "interval", interval)
	} else {
		slog.Debug("periodic MCP schema refresh disabled (mcp.schema_refresh_interval unset)")
	}

	var healthChecker *cache.HealthChecker
	if interval := cfg.HealthCheckInterval(); interval > 0 {
		healthChecker = cache.NewHealthChecker(registry, interval)
		slog.Info("MCP health check enabled", "interval", interval)
	} else {
		slog.Warn("MCP health check is DISABLED (mcp.health_check_interval = 0) — " +
			"a downstream MCP that restarts will stay broken until this gateway is restarted (ADR-0019)")
	}

	return &App{
		Gateway:       gateway,
		ControlPlane:  controlPlane,
		refresher:     refresher,
		healthChecker: healthChecker,
	}, nil
}

// Run starts both HTTP surfaces and blocks until ctx is done, then
// gracefully shuts both down (draining in-flight requests, up to
// shutdownTimeout) before returning. A start failure on either surface
// causes Run to return promptly without waiting for ctx.
func (a *App) Run(ctx context.Context) error {
	dataPlaneErr := make(chan error, 1)
	controlPlaneErr := make(chan error, 1)
	refresherErr := make(chan error, 1)
	healthErr := make(chan error, 1)

	go func() { dataPlaneErr <- a.Gateway.Start(ctx) }()
	go func() { controlPlaneErr <- a.ControlPlane.Start(ctx) }()
	if a.refresher != nil {
		go func() { refresherErr <- a.refresher.Run(ctx) }()
	} else {
		refresherErr <- nil
	}
	if a.healthChecker != nil {
		go func() { healthErr <- a.healthChecker.Run(ctx) }()
	} else {
		healthErr <- nil
	}

	select {
	case <-a.Gateway.Ready():
	case err := <-dataPlaneErr:
		return fmt.Errorf("app: data plane failed to start: %w", err)
	case <-ctx.Done():
		return a.shutdown(dataPlaneErr, controlPlaneErr, refresherErr, healthErr)
	}

	select {
	case <-a.ControlPlane.Ready():
	case err := <-controlPlaneErr:
		return fmt.Errorf("app: control plane failed to start: %w", err)
	case <-ctx.Done():
		return a.shutdown(dataPlaneErr, controlPlaneErr, refresherErr, healthErr)
	}

	<-ctx.Done()
	return a.shutdown(dataPlaneErr, controlPlaneErr, refresherErr, healthErr)
}

// shutdown gracefully stops both surfaces and waits for their Start calls
// (and the two background loops) to return, joining any errors encountered.
// Both loops stop on their own once ctx is done — the same trigger — so
// there is nothing to Stop explicitly.
func (a *App) shutdown(dataPlaneErr, controlPlaneErr, refresherErr, healthErr <-chan error) error {
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
	if err := <-refresherErr; err != nil {
		errs = append(errs, err)
	}
	if err := <-healthErr; err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// adminAuthInfo builds what GET /admin/auth/config reports, from the same
// config the gate itself is built from. Only reached when admin auth is
// enabled; with it off the zero AuthInfo ("auth_required": false) is what
// the UI should see. See ADR-0014.
func adminAuthInfo(cfg *config.Config) controlplane.AuthInfo {
	info := controlplane.AuthInfo{AuthRequired: true, Issuer: cfg.OIDC.Issuer}
	login, ok := cfg.AdminLogin()
	if !ok {
		return info
	}
	info.ClientID = login.ClientID
	info.AuthorizationEndpoint = login.AuthorizationEndpoint
	info.TokenEndpoint = login.TokenEndpoint
	info.Scopes = login.ScopesOrDefault()
	return info
}
