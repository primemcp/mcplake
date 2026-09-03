package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	port := flag.Int("port", 8080, "Port for the gateway to listen on")
	uiPort := flag.Int("ui-port", 8081, "Port for the admin UI")
	flag.Parse()

	log.Printf("mcplake gateway starting...")
	log.Printf("Configuration: %s", *configPath)
	log.Printf("Gateway listening on: %d", *port)
	log.Printf("Admin UI listening on: %d", *uiPort)

	// TODO: Initialize configuration
	// cfg, err := config.Load(*configPath)
	// if err != nil {
	//     log.Fatalf("Failed to load configuration: %v", err)
	// }

	// TODO: Initialize OIDC provider and cache JWKS
	// oidcValidator, err := auth.NewValidator(cfg.OIDC)

	// TODO: Initialize MCP schema cache
	// schemaCache, err := cache.NewSchemaCache(cfg.MCPs)

	// TODO: Initialize router
	// routerInstance, err := router.NewRouter(cfg.Routing)

	// TODO: Initialize response filter
	// filterInstance, err := filter.NewFilter(cfg.Filtering)

	// TODO: Start gateway server
	// gw, err := gateway.NewGateway(cfg, oidcValidator, schemaCache, routerInstance, filterInstance)

	// TODO: Start admin UI
	// ui, err := ui.NewServer(cfg.UI, gw)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal: %v, shutting down...\n", sig)
		// TODO: Gracefully shutdown gateway
		// TODO: Gracefully shutdown UI
		os.Exit(0)
	}()

	// Block until shutdown
	select {}
}
