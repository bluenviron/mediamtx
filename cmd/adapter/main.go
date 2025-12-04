// Package main provides the entry point for the FPT Camera WebRTC Adapter
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bluenviron/mediamtx/internal/adapter"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Command line flags
	showVersion := flag.Bool("version", false, "Show version information")
	showEnvExample := flag.Bool("env-example", false, "Show example environment variables")
	configFile := flag.String("config", "", "Path to configuration file (optional)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("FPT Camera WebRTC Adapter v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	if *showEnvExample {
		fmt.Println(adapter.EnvExample())
		os.Exit(0)
	}

	// Load configuration
	config := adapter.LoadConfigFromEnv()
	
	// Override with config file if provided
	if *configFile != "" {
		// TODO: Implement config file loading
		fmt.Printf("Config file loading not yet implemented: %s\n", *configFile)
	}

	fmt.Println("===========================================")
	fmt.Println("  FPT Camera WebRTC Adapter")
	fmt.Printf("  Version: %s\n", version)
	fmt.Println("===========================================")
	fmt.Println()

	// Create and start adapter
	adapterInstance := adapter.NewAdapter(config)

	// Set callbacks
	adapterInstance.OnClientConnected = func(clientID, serial string) {
		fmt.Printf("[INFO] Client connected: %s (camera: %s)\n", clientID, serial)
	}

	adapterInstance.OnClientDisconnected = func(clientID, serial string) {
		fmt.Printf("[INFO] Client disconnected: %s (camera: %s)\n", clientID, serial)
	}

	adapterInstance.OnError = func(err error) {
		fmt.Printf("[ERROR] %v\n", err)
	}

	// Start the adapter
	if err := adapterInstance.Start(); err != nil {
		fmt.Printf("[FATAL] Failed to start adapter: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[INFO] Adapter started successfully")
	fmt.Printf("[INFO] MQTT Broker: %s\n", config.MQTT.BrokerURL)
	fmt.Printf("[INFO] MediaMTX WHEP: %s\n", config.WHEP.BaseURL)
	fmt.Printf("[INFO] Max Sessions: %d\n", config.MaxSessions)
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop...")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println()
	fmt.Println("[INFO] Shutting down...")
	adapterInstance.Stop()
	fmt.Println("[INFO] Adapter stopped")
}
