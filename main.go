// Package main provides the entry point for ratox-go, a FIFO-based Tox client
// that implements the same filesystem interface as the original ratox client.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/opd-ai/go-ratox/client"
	"github.com/opd-ai/go-ratox/config"
)

const (
	// DefaultConfigDir is the default directory for ratox configuration
	DefaultConfigDir = ".config/ratox-go"
	// Version of the ratox-go client
	Version = "0.1.0"
)

var (
	configPath = flag.String("p", "", "Path to configuration directory")
	showHelp   = flag.Bool("h", false, "Show help message")
	showVer    = flag.Bool("v", false, "Show version")
	debug      = flag.Bool("d", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	if *showHelp {
		printUsage()
		return
	}

	if *showVer {
		fmt.Printf("ratox-go %s\n", Version)
		return
	}

	// Determine configuration directory
	configDir := *configPath
	if configDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get home directory: %v", err)
		}
		configDir = filepath.Join(homeDir, DefaultConfigDir)
	}

	// Create configuration directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	// Load or create configuration
	cfg, err := config.Load(configDir)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Enable debug logging if requested
	if *debug {
		cfg.Debug = true
	}

	// Create and start the Tox client
	toxClient, err := client.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create Tox client: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the client in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- toxClient.Run()
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Client error: %v", err)
		}
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
		toxClient.Shutdown()
	}

	log.Println("ratox-go shutdown complete")
}

func printUsage() {
	fmt.Printf("ratox-go %s - FIFO-based Tox client\n\n", Version)
	fmt.Println("Usage:")
	fmt.Printf("  %s [options]\n\n", os.Args[0])
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println("\nExamples:")
	fmt.Printf("  %s -p ~/.config/ratox-go\n", os.Args[0])
	fmt.Printf("  %s -d  # Enable debug logging\n", os.Args[0])
	fmt.Println("\nFileSystem Interface:")
	fmt.Println("  ~/.config/ratox-go/")
	fmt.Println("  ├── <friend_id>/")
	fmt.Println("  │   ├── text_in     # Send messages")
	fmt.Println("  │   ├── text_out    # Receive messages")
	fmt.Println("  │   ├── file_in     # Send files")
	fmt.Println("  │   ├── file_out    # Receive files")
	fmt.Println("  │   └── status      # Friend status")
	fmt.Println("  ├── request_in      # Accept friend requests")
	fmt.Println("  ├── request_out     # Incoming friend requests")
	fmt.Println("  ├── name            # Your display name")
	fmt.Println("  └── status_message  # Your status message")
}
