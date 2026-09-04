package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/atsokha/mcplake/cmd/gateway/app"
	"github.com/atsokha/mcplake/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("mcplake: load config %s: %v", *configPath, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a, err := app.New(ctx, cfg)
	if err != nil {
		log.Fatalf("mcplake: initialize: %v", err)
	}

	slog.Info("mcplake gateway starting",
		"data_plane_addr", cfg.Server.DataPlaneAddr,
		"control_plane_addr", cfg.Server.ControlPlaneAddr,
	)

	if err := a.Run(ctx); err != nil {
		log.Fatalf("mcplake: %v", err)
	}
}
