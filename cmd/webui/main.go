package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Defaults are chosen for the container this runs in (deploy/webui/
// Dockerfile), not for a developer's laptop: the image copies the built SPA
// to /srv/www, and compose puts the gateway's control plane on the address
// below.
const (
	defaultAddr      = ":8081"
	defaultRoot      = "/srv/www"
	defaultAPIPrefix = "/admin"
	// shutdownTimeout bounds how long Run waits for in-flight requests.
	shutdownTimeout = 10 * time.Second
	// readHeaderTimeout bounds a client that opens a connection and then
	// dawdles over the request line; without it a handful of idle sockets
	// can hold the server open. Everything else here is served from memory
	// or proxied, so no whole-request deadline is wanted.
	readHeaderTimeout = 10 * time.Second
)

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Addr, "addr", envOr("WEBUI_ADDR", defaultAddr),
		"listen address")
	flag.StringVar(&cfg.Root, "root", envOr("WEBUI_ROOT", defaultRoot),
		"directory holding the built SPA")
	flag.StringVar(&cfg.APIPrefix, "api-prefix", envOr("WEBUI_API_PREFIX", defaultAPIPrefix),
		"path prefix reverse-proxied to the gateway control plane")
	flag.StringVar(&cfg.APITarget, "api-target", envOr("WEBUI_API_TARGET", ""),
		"gateway control-plane base URL, e.g. http://localhost:9091 (empty disables proxying)")
	flag.Parse()

	if err := run(cfg); err != nil {
		slog.Error("webui server failed", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func run(cfg Config) error {
	handler, err := newHandler(cfg)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	if cfg.APITarget == "" {
		slog.Warn("admin API proxying is disabled (no -api-target) — " +
			"the UI's relative /admin calls will 404 unless something in front of this routes them")
	}
	slog.Info("mcplake admin UI server starting",
		"addr", cfg.Addr, "root", cfg.Root,
		"api_prefix", cfg.APIPrefix, "api_target", cfg.APITarget)

	errs := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("webui: listen on %s: %w", cfg.Addr, err)
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("webui: shutdown: %w", err)
	}
	return <-errs
}
