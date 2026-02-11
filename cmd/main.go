package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/seds-net/seds-agent/config"
	"github.com/seds-net/seds-agent/grpc"
	"github.com/seds-net/seds-agent/singbox"
)

var (
	configPath  = flag.String("config", "config.yaml", "Path to configuration file")
	genConfig   = flag.Bool("gen-config", false, "Generate example configuration file")
	server      = flag.String("server", "", "Override server address (host:port)")
	token       = flag.String("token", "", "Override authentication token")
	singboxPath = flag.String("singbox", "", "Override sing-box executable path")
	version     = "dev"
)

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("seds-agent version: %s", version)

	// Generate example config and exit
	if *genConfig {
		if err := config.GenerateExample(*configPath); err != nil {
			log.Fatalf("Failed to generate config: %v", err)
		}
		log.Printf("Example configuration generated at: %s", *configPath)
		return
	}

	// Load configuration
	if err := config.Load(*configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cfg := config.Get()

	// Override with command-line flags
	if *server != "" {
		cfg.Server = *server
	}
	if *token != "" {
		cfg.Token = *token
	}
	if *singboxPath != "" {
		cfg.SingBoxPath = *singboxPath
	}

	// Validate required configuration
	if cfg.Server == "" {
		log.Fatal("Server address is required (set in config or use -server flag)")
	}
	if cfg.Token == "" {
		log.Fatal("Authentication token is required (set in config or use -token flag)")
	}

	log.Printf("Server: %s", cfg.Server)
	log.Printf("Config directory: %s", cfg.ConfigDir)
	log.Printf("Sing-box path: %s", cfg.SingBoxPath)

	// Initialize sing-box manager
	sbManager := singbox.NewManager(cfg.SingBoxPath, cfg.ConfigDir)

	// Initialize gRPC client
	client := grpc.NewClient(sbManager)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")

		// Stop sing-box
		if sbManager.IsRunning() {
			if err := sbManager.Stop(); err != nil {
				log.Printf("Error stopping sing-box: %v", err)
			}
		}

		// Close gRPC connection
		if err := client.Close(); err != nil {
			log.Printf("Error closing client: %v", err)
		}

		os.Exit(0)
	}()

	log.Println("Starting agent...")

	// Run client (with auto-reconnection)
	client.Run()
}
