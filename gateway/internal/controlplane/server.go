// Package controlplane implements the Gin-based admin API server: the
// control-plane HTTP surface for MCP registration and access/filter policy
// CRUD, separate from the fasthttp data plane in package internal. See
// docs/architecture/decisions/0005-use-gin-for-control-plane-api.rst.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	_ "github.com/atsokha/mcplake/internal/controlplane/docs"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// idleTimeout bounds how long a keep-alive connection may sit idle, so a
// client holding a persistent connection open can't keep graceful shutdown
// from completing — mirrors internal.Gateway's idleTimeout rationale.
const idleTimeout = 60 * time.Second

// Config configures the control-plane Server.
type Config struct {
	// ControlPlaneAddr is the listen address (host:port) for the admin API,
	// e.g. ":8081". See config.ServerConfig.ControlPlaneAddr.
	ControlPlaneAddr string

	// AdminAuth, when non-nil, is installed as middleware on the /admin
	// group ahead of every route except GET /admin/healthz — so the health
	// check stays open while MCP/policy CRUD, the MCP control server, and
	// the Swagger UI all require an admin JWT. Build it with AdminAuth().
	// When nil the admin API is unauthenticated (see ADR-0010); the caller
	// is responsible for logging that.
	AdminAuth gin.HandlerFunc

	// AuthInfo is served at GET /admin/auth/config, unauthenticated, so the
	// embedded web UI can discover whether and how to log in. See ADR-0014
	// and AuthInfo's own doc comment for why leaving it open is safe. The
	// zero value reports "no auth required", matching a nil AdminAuth.
	AuthInfo AuthInfo
}

// Server is the Gin-based control-plane admin API server: its own listener,
// independent of the data-plane fasthttp server. It establishes the
// "/admin" route group and a health endpoint; tickets #47-#49 register the
// actual CRUD routes onto the group returned by Admin.
//
// Server must not be copied after first use.
type Server struct {
	addr       string
	engine     *gin.Engine
	admin      *gin.RouterGroup
	httpServer *http.Server

	mu    sync.Mutex
	ln    net.Listener
	ready chan struct{}
	once  sync.Once
}

// NewServer constructs a Server bound to cfg.ControlPlaneAddr. It does not
// start listening; call Start to do that.
func NewServer(cfg Config) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	// Route on the escaped path, so a name containing "/" -- which POST
	// accepts -- still fits one :name segment when sent as %2F, and is
	// decoded into the parameter (UnescapePathValues is on by default).
	// Routing on the decoded path split it in two and every per-name
	// PATCH/PUT/DELETE 404'd, so such an entry could be created but never
	// removed. See #205.
	engine.UseRawPath = true
	engine.Use(gin.Recovery())

	admin := engine.Group("/admin")
	// Ahead of everything, including admin auth: a body-carrying request
	// that does not declare JSON is refused before any handler or
	// authenticator sees it. This is what stops a cross-origin
	// "simple request" from reaching a write handler -- see
	// requireJSONContentType.
	admin.Use(requireJSONContentType())
	// Registered ahead of cfg.AdminAuth, so both stay reachable without a
	// token: healthz for liveness probes, auth/config so a browser that has
	// no token yet can find out how to get one (ADR-0014).
	admin.GET("/healthz", healthz)
	admin.GET("/auth/config", authConfig(cfg.AuthInfo))
	if cfg.AdminAuth != nil {
		admin.Use(cfg.AdminAuth)
	}
	admin.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	s := &Server{
		addr:   cfg.ControlPlaneAddr,
		engine: engine,
		admin:  admin,
		ready:  make(chan struct{}),
	}
	s.httpServer = &http.Server{
		Handler:     engine,
		IdleTimeout: idleTimeout,
	}
	return s
}

// Admin returns the "/admin" route group, so callers (the admin API
// handlers added in later tickets) can register additional routes onto the
// same server/engine.
func (s *Server) Admin() *gin.RouterGroup {
	return s.admin
}

// Engine returns the underlying Gin engine, so callers can register routes
// outside the "/admin" group onto the same server/listener — e.g.
// RegisterUIRoutes, which serves the admin web UI from this server's
// ControlPlaneAddr instead of a separate listener (see the Admin UI epic,
// #76).
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// Ready returns a channel that is closed once the Server has bound its
// listener and is accepting connections.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// Addr returns the actual bound address, useful when Config.ControlPlaneAddr
// used an ephemeral port (":0") such as in tests. It is empty until Start
// has bound the listener.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Start binds the listener and serves requests until the server is shut down
// via Stop, in which case Start returns nil, or an unrecoverable server
// error occurs.
func (s *Server) Start(_ context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("controlplane: listen on %s: %w", s.addr, err)
	}

	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	s.once.Do(func() { close(s.ready) })

	err = s.httpServer.Serve(ln)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("controlplane: serve: %w", err)
	}
	return nil
}

// Stop gracefully shuts the server down: it stops accepting new connections
// and waits for in-flight requests to finish, up to ctx's deadline. It
// unblocks the goroutine running Start.
func (s *Server) Stop(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("controlplane: shutdown: %w", err)
	}
	return nil
}

// @Summary  Control-plane health check
// @Tags     health
// @Produce  plain
// @Success  200  {string}  string  "ok"
// @Router   /healthz [get]
func healthz(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}
